package vfs_service

import (
	"fmt"
	"testing"

	cvelo_services "www.velocidex.com/golang/cloudvelo/services"
	"www.velocidex.com/golang/velociraptor/utils"
)

func TestFoo(t *testing.T) {
	dir_components := []string{
		"C:",
		"28f56b8f3bd7cad47a107c29b62fe066",
		"auto",
		"C:",
	}

	id := cvelo_services.MakeId(utils.JoinComponents(dir_components, "/"))
	fmt.Printf("Id should be %v %v\n", id, utils.JoinComponents(dir_components, "/"))
}
