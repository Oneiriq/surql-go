package settings

import "gopkg.in/yaml.v3"

// unmarshalYAML is a thin wrapper over yaml.v3 so yaml.go is the only
// file that imports the YAML codec.
func unmarshalYAML(data []byte, out any) error {
	return yaml.Unmarshal(data, out)
}
