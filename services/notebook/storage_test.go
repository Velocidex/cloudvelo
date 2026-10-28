package notebook_test

import (
	"testing"

	"github.com/alecthomas/assert"
	"github.com/stretchr/testify/suite"
	"www.velocidex.com/golang/cloudvelo/services/notebook"
	"www.velocidex.com/golang/cloudvelo/testsuite"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
)

const (
	testNotebookId = "N.TESTNOTEBOOK"
	testCellId     = "NC.TESTCELL"
)

type NotebookStoreTestSuite struct {
	*testsuite.CloudTestSuite

	store *notebook.NotebookStoreImpl
}

func (self *NotebookStoreTestSuite) SetupTest() {
	self.CloudTestSuite.SetupTest()

	self.store = notebook.NewNotebookStore(
		self.Ctx, self.Sm.Wg, self.ConfigObj.VeloConf(), nil)

	err := self.store.SetNotebook(&api_proto.NotebookMetadata{
		NotebookId: testNotebookId,
		Creator:    "admin",
	})
	assert.NoError(self.T(), err)
}

func (self *NotebookStoreTestSuite) TestNewCellSummary() {
	err := self.store.SetNotebookCell(testNotebookId, &api_proto.NotebookCell{
		CellId:            testCellId,
		CurrentVersion:    "01",
		AvailableVersions: []string{"01"},
		Type:              "markdown",
		Output:            "<h1>Hello</h1>",
		Calculating:       true,
	})
	assert.NoError(self.T(), err)

	cell_md := self.getCellMetadata()
	assert.Equal(self.T(), 1, len(cell_md))
	assert.Equal(self.T(), testCellId, cell_md[0].CellId)
	assert.Equal(self.T(), "Hello", cell_md[0].Summary)
	assert.True(self.T(), cell_md[0].Calculating)
	assert.Equal(self.T(), "", cell_md[0].Output)
}

func (self *NotebookStoreTestSuite) TestExistingCellSummary() {
	err := self.store.SetNotebookCell(testNotebookId, &api_proto.NotebookCell{
		CellId:            testCellId,
		CurrentVersion:    "01",
		AvailableVersions: []string{"01"},
		Type:              "markdown",
		Output:            "<h1>Hello</h1>",
		Calculating:       true,
	})
	assert.NoError(self.T(), err)

	err = self.store.SetNotebookCell(testNotebookId, &api_proto.NotebookCell{
		CellId:            testCellId,
		CurrentVersion:    "02",
		AvailableVersions: []string{"01", "02"},
		Type:              "markdown",
		Output:            "<h2>Updated</h2>",
		Calculating:       false,
	})
	assert.NoError(self.T(), err)

	cell_md := self.getCellMetadata()
	assert.Equal(self.T(), 1, len(cell_md))
	assert.Equal(self.T(), "Updated", cell_md[0].Summary)
	assert.False(self.T(), cell_md[0].Calculating)
	assert.Equal(self.T(), "02", cell_md[0].CurrentVersion)
	assert.Equal(self.T(), []string{"01", "02"}, cell_md[0].AvailableVersions)
}

func (self *NotebookStoreTestSuite) TestCellWithoutHeadingHasNoSummary() {
	err := self.store.SetNotebookCell(testNotebookId, &api_proto.NotebookCell{
		CellId:            testCellId,
		CurrentVersion:    "01",
		AvailableVersions: []string{"01"},
		Type:              "markdown",
		Output:            "<p>plain</p>",
	})
	assert.NoError(self.T(), err)

	cell_md := self.getCellMetadata()
	assert.Equal(self.T(), 1, len(cell_md))
	assert.Equal(self.T(), "", cell_md[0].Summary)
}

func TestNotebookStore(t *testing.T) {
	suite.Run(t, &NotebookStoreTestSuite{
		CloudTestSuite: &testsuite.CloudTestSuite{},
	})
}

func (self *NotebookStoreTestSuite) getCellMetadata() []*api_proto.NotebookCell {
	notebook_md, err := self.store.GetNotebook(testNotebookId)
	assert.NoError(self.T(), err)
	return notebook_md.CellMetadata
}
