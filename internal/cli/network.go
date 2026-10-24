package cli

import "fmt"

func (s *Session) network(w []string) (string, error) {
	n := s.Candidate.Networks[s.Object]
	if configureNetworkOptions(&n, w) {
		s.Candidate.Networks[s.Object] = n
		return "", nil
	}
	if len(w) == 2 {
		switch w[0] {
		case "driver":
			n.Driver = w[1]
		case "subnet":
			n.Subnet = w[1]
		case "gateway":
			n.Gateway = w[1]
		case "parent":
			if s.InterfaceName != nil {
				name, err := s.InterfaceName(w[1])
				if err != nil {
					return "", err
				}
				w[1] = name
			}
			n.Parent = w[1]
		case "no":
			switch w[1] {
			case "internal":
				n.Internal = false
			case "attachable":
				n.Attachable = false
			case "gateway":
				n.Gateway = ""
			case "subnet":
				n.Subnet = ""
			case "parent":
				n.Parent = ""
			default:
				return "", fmt.Errorf("invalid network setting")
			}
		default:
			return "", fmt.Errorf("invalid network setting")
		}
	} else if len(w) == 1 && w[0] == "internal" {
		n.Internal = true
	} else if len(w) == 1 && w[0] == "attachable" {
		n.Attachable = true
	} else {
		return "", fmt.Errorf("invalid network setting")
	}
	s.Candidate.Networks[s.Object] = n
	return "", nil
}
