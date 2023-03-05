package users

import (
	"context"
	"errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
)

var (
	deleteUserCounter = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "velociraptor_delete_user_count",
			Help: "Count of users deleted from the Velociraptor system.",
		})
)

func (self *UserManager) DeleteUser(
	ctx context.Context, org_config_obj *config_proto.Config, username string) error {

	logger := logging.GetLogger(&self.config_obj.Config, &logging.Audit)
	if logger != nil {
		logger.Info("Deleted user: %v", username)
	}

	deleteUserCounter.Inc()

	return errors.New("UserManager.DeleteUser not implemented")
}
