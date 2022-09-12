package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"www.velocidex.com/golang/cloudvelo/config"
	"www.velocidex.com/golang/cloudvelo/crypto/server"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

type CommunicatorBackend interface {
	Send(ctx context.Context, messages []*crypto_proto.VeloMessage) error
	Receive(ctx context.Context, client_id, org_id string) (
		messages []*crypto_proto.VeloMessage,
		org_config_obj *config_proto.Config, err error)
}

type Communicator struct {
	config_obj *config.Config
	client_id  string
	backend    CommunicatorBackend

	// This is just for testing so we can hold all parts in memory and
	// flush to disk when done.
	mu sync.Mutex

	session *session.Session

	parts []*s3.CompletedPart

	crypto_manager *server.ServerCryptoManager
}

// Receive a POST message from the client with the VeloMessage in
// it. This handler is for communication FROM clients TO server
func (self Communicator) Send(w http.ResponseWriter, r *http.Request) {
	serialized, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "", http.StatusForbidden)
		return
	}

	message_info, err := self.crypto_manager.Decrypt(serialized)
	if err != nil {
		// Just plain reject with a 403.
		http.Error(w, "", http.StatusForbidden)
		return
	}
	message_info.RemoteAddr = r.RemoteAddr
	logger := logging.GetLogger(
		self.config_obj.VeloConf(), &logging.FrontendComponent)

	err = message_info.IterateJobs(r.Context(), func(
		ctx context.Context, message *crypto_proto.VeloMessage) {
		err := self.backend.Send(r.Context(), []*crypto_proto.VeloMessage{message})
		if err != nil {
			logger.Error("Communicator.Send: %v", err)
		}
	})
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		logger.Error("Unable to send to backend: %v", err)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// Serve the receive handler - retrieve messages from the
// backend. This handler is for communication FROM server TO client
func (self Communicator) Receive(w http.ResponseWriter, r *http.Request) {
	// Only support a GET method
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	body, err := ioutil.ReadAll(
		io.LimitReader(r.Body, constants.MAX_MEMORY))
	if err != nil {
		logger := logging.GetLogger(
			self.config_obj.VeloConf(), &logging.FrontendComponent)
		logger.Error("Unable to read body: %v", err)
		http.Error(w, "", http.StatusForbidden)
		return
	}

	message_info, err := self.crypto_manager.Decrypt(body)
	if err != nil {
		// Just plain reject with a 403.
		http.Error(w, "", http.StatusForbidden)
		return
	}
	message_info.RemoteAddr = r.RemoteAddr

	// Reject unauthenticated messages. This ensures
	// untrusted clients are not allowed to keep
	// connections open.
	if !message_info.Authenticated {
		http.Error(w, "Please Enrol", http.StatusNotAcceptable)
		return
	}

	// Process Foreman ping messages to update the client's last seen
	// time.
	err = message_info.IterateJobs(r.Context(), func(
		ctx context.Context, message *crypto_proto.VeloMessage) {
		self.backend.Send(r.Context(), []*crypto_proto.VeloMessage{message})
	})
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		logger := logging.GetLogger(
			self.config_obj.VeloConf(), &logging.FrontendComponent)
		logger.Error("Unable to send to backend: %v", err)
		return
	}

	client_id := message_info.Source
	org_id := message_info.OrgId

	messages, org_config_obj, err := self.backend.Receive(r.Context(),
		client_id, org_id)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(err.Error()))
		return
	}

	message_list := &crypto_proto.MessageList{
		Job: messages,
	}

	response, err := self.crypto_manager.EncryptMessageList(
		message_list,
		crypto_proto.PackedMessageList_UNCOMPRESSED,
		org_config_obj.Client.Nonce,
		message_info.Source)
	if err != nil {
		http.Error(w, "Please Enrol", http.StatusNotAcceptable)
		return
	}

	w.Write(response)
}

func (self Communicator) Start(
	ctx context.Context,
	config_obj *config_proto.Config,
	wg *sync.WaitGroup) error {

	mux := http.NewServeMux()

	// A POST to this URL will send a VeloMessage in the body.
	mux.HandleFunc("/control", self.Send)

	// A GET to this URL will return the VeloMessage destined to the client.
	mux.HandleFunc("/reader", self.Receive)

	mux.Handle("/server.pem", server_pem(config_obj))

	// The following end points will allow file uploads to S3. A POST
	// to Start will produce an upload key that can be used to upload
	// multiple parts using the PUT HTTP Method. The file may be
	// finalized using Commit, or aborted using the abort handler.
	mux.HandleFunc("/upload/start", self.StartMultipartUpload)
	mux.HandleFunc("/upload/put", self.GetUploadPart)
	mux.HandleFunc("/upload/commit", self.CompleteMultipartUpload)
	mux.HandleFunc("/upload/abort", self.AbortMultipartUpload)

	certs, err := getCertificates(config_obj)
	if err != nil {
		return err
	}

	expected_clients := int64(10000)
	if config_obj.Frontend != nil &&
		config_obj.Frontend.Resources != nil &&
		config_obj.Frontend.Resources.ExpectedClients > 0 {
		expected_clients = config_obj.Frontend.Resources.ExpectedClients
	}

	logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
	logger.Info("Starting frontend communicator on port %d",
		config_obj.Frontend.BindPort)

	listenAddr := fmt.Sprintf(
		"%s:%d",
		config_obj.Frontend.BindAddress,
		config_obj.Frontend.BindPort)

	http_server := &http.Server{
		Addr:           listenAddr,
		Handler:        mux,
		MaxHeaderBytes: 1 << 20,
		ErrorLog:       logging.NewPlainLogger(config_obj, &logging.FrontendComponent),

		// https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
		ReadTimeout:  500 * time.Second,
		WriteTimeout: 900 * time.Second,
		IdleTimeout:  150 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			ClientSessionCache: tls.NewLRUClientSessionCache(int(expected_clients)),
			Certificates:       certs,
			CurvePreferences: []tls.CurveID{tls.CurveP521,
				tls.CurveP384, tls.CurveP256},

			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_RSA_WITH_AES_256_CBC_SHA,
				tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			},
		},
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()

		logger.Info("<red>Shutting down</> frontend")
		time_ctx, cancel := context.WithTimeout(
			context.Background(), 10*time.Second)
		defer cancel()

		http_server.Shutdown(time_ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		logger.Info("Frontend is ready to handle client TLS requests on %v",
			http_server.Addr)
		ln, err := net.Listen("tcp", http_server.Addr)
		if err != nil {
			return
		}

		err = http_server.ServeTLS(ln, "", "")
		if err != nil && err != http.ErrServerClosed {
			logger.Error("Frontend server error %v", err)
			return
		}
	}()

	return nil
}

func getCertificates(config_obj *config_proto.Config) ([]tls.Certificate, error) {
	// If we need to read TLS certs from a file then do it now.
	if config_obj.Frontend.TlsCertificateFilename != "" {
		cert, err := tls.LoadX509KeyPair(
			config_obj.Frontend.TlsCertificateFilename,
			config_obj.Frontend.TlsPrivateKeyFilename)
		if err != nil {
			return nil, err
		}
		return []tls.Certificate{cert}, nil
	}

	cert, err := tls.X509KeyPair(
		[]byte(config_obj.Frontend.Certificate),
		[]byte(config_obj.Frontend.PrivateKey))
	if err != nil {
		return nil, err
	}

	return []tls.Certificate{cert}, nil
}

func server_pem(config_obj *config_proto.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)
		flusher.Flush()

		_, _ = w.Write([]byte(config_obj.Frontend.Certificate))
	})
}
