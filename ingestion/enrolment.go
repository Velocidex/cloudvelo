package ingestion

import (
	"context"

	"www.velocidex.com/golang/cloudvelo/services"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
)

func (self Ingestor) HandleEnrolment(
	config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	csr := message.CSR
	if csr == nil {
		return nil
	}

	client_id, err := self.crypto_manager.AddCertificateRequest(config_obj, csr.Pem)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.FrontendComponent)
		logger.Error("While enrolling %v: %v", client_id, err)
		return err
	}

	return nil
}

const (
	updateClientInterrogate = `
{
  "script" : {
    "source": "ctx._source.last_interrogate = params.last_interrogate",
    "lang": "painless",
    "params": {
      "last_interrogate": %q
    }
  }
}
`
)

func (self Ingestor) HandleInterrogation(
	ctx context.Context, config_obj *config_proto.Config,
	message *crypto_proto.VeloMessage) error {

	query := json.Format(updateClientInterrogate, message.SessionId)
	return services.UpdateIndex(
		ctx, config_obj.OrgId, "clients", message.Source, query)
}
