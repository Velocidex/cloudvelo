package security

import (
	"runtime"

	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/accessors/file"
	"www.velocidex.com/golang/velociraptor/accessors/file_store"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/common"
)

// Paths denied to the fs accessor when the config denies none.
var default_denied_fs_accessor_prefix = []string{
	"acl",
	"backups",
	"config",
	"orgs",
	"secrets",
	"users",
}

// Apply builds the file and fs accessor prefix trees
// and the shadowed environment variables from the Security config
// block. Upstream does this in the sanity service, which we do not run.
func Apply(config_obj *config_proto.Config) error {
	security := config_obj.Security
	if security == nil {
		security = &config_proto.Security{}
	}

	// On windows we must use case insensitive match.
	case_insensitive := runtime.GOOS == "windows"

	file.SetPrefixes(
		buildPrefixTree(security.AllowedFileAccessorPrefix,
			case_insensitive, accessors.NewNativePath),
		buildPrefixTree(security.DeniedFileAccessorPrefix,
			case_insensitive, accessors.NewNativePath))

	denied_fs_prefixes := security.DeniedFsAccessorPrefix
	if len(denied_fs_prefixes) == 0 {
		denied_fs_prefixes = default_denied_fs_accessor_prefix
	}

	file_store.SetPrefixes(
		buildPrefixTree(security.AllowedFsAccessorPrefix,
			false, accessors.NewFileStorePath),
		buildPrefixTree(denied_fs_prefixes,
			false, accessors.NewFileStorePath))

	for _, s := range security.ShadowedEnvVars {
		if !utils.InString(common.ShadowedEnv, s) {
			common.ShadowedEnv = append(common.ShadowedEnv, s)
		}
	}

	return nil
}

// An empty list gives a nil tree, which places no restriction.
func buildPrefixTree(prefixes []string, case_insensitive bool,
	parse func(string) (*accessors.OSPath, error)) *utils.PrefixTree {
	if len(prefixes) == 0 {
		return nil
	}

	tree := utils.NewPrefixTree(case_insensitive)
	for _, prefix := range prefixes {
		full_path, err := parse(prefix)
		if err != nil {
			continue
		}
		tree.Add(full_path.Components)
	}
	return tree
}
