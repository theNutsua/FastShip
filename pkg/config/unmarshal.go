package config

import "gopkg.in/yaml.v3"

// UnmarshalYAML lets a Service be written as either a bare string or a
// full object. Without this, "- postgres" would fail to parse into a
// struct and engineers would be forced to write "- name: postgres".
// yaml.v3 calls this with the raw node, so we can inspect its kind
// before deciding how to decode it.
func (s *Service) UnmarshalYAML(node *yaml.Node) error {
	// Scalar means a bare value: "- postgres"
	if node.Kind == yaml.ScalarNode {
		s.Name = node.Value
		return nil
	}

	// Otherwise decode as a struct. The alias type is essential: decoding
	// into *Service directly would call this method again and recurse
	// until the stack blows. serviceAlias has no UnmarshalYAML method,
	// so yaml.v3 uses its default struct decoding.
	type serviceAlias Service
	var alias serviceAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}
	*s = Service(alias)
	return nil
}

// UnmarshalYAML lets a Secret be written as either a bare name or an
// object naming an external provider. Same recursion guard as above.
func (s *Secret) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		s.Name = node.Value
		return nil
	}

	type secretAlias Secret
	var alias secretAlias
	if err := node.Decode(&alias); err != nil {
		return err
	}
	*s = Secret(alias)
	return nil
}
