package labeler_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

type LabelerTestSuite struct {
	*testsuite.CloudTestSuite
}

// SetClientLabel creates the labels document if it is missing, so
// without validation any string - including "" - gets one.
func (self *LabelerTestSuite) TestSetClientLabelRejectsInvalidClientIds() {
	config_obj := self.ConfigObj.VeloConf()

	labeler := services.GetLabeler(config_obj)

	for _, client_id := range []string{"", "notaclient"} {
		err := labeler.SetClientLabel(self.Ctx, config_obj, client_id, "Rejected")
		assert.Error(self.T(), err, "client id %q", client_id)
	}

	count, err := cvelo_services.QueryCountAPI(self.Ctx, self.OrgId, "persisted",
		`{"query": {"term": {"lower_labels": "rejected"}}}`)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 0, count)

	err = labeler.SetClientLabel(self.Ctx, config_obj, "C.1234", "Accepted")
	assert.NoError(self.T(), err)

	count, err = cvelo_services.QueryCountAPI(self.Ctx, self.OrgId, "persisted",
		`{"query": {"term": {"lower_labels": "accepted"}}}`)
	assert.NoError(self.T(), err)
	assert.Equal(self.T(), 1, count)
}

func TestLabeler(t *testing.T) {
	suite.Run(t, &LabelerTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{},
	})
}
