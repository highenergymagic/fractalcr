package cli

func init() {
	for _, prefix := range []string{"username <NAME>", "username <NAME> privilege <0-15>", "enable", "enable secret level <1-15>"} {
		secretPrefix := prefix + " secret"
		if prefix == "enable secret level <1-15>" {
			secretPrefix = prefix
		}
		for _, kind := range []string{"8", "9"} {
			configCommands = append(configCommands, Command{secretPrefix + " " + kind + " <SECRET...>", "Import an encoded secret", false})
		}
		if prefix != "enable secret level <1-15>" {
			for _, algorithm := range []string{"sha256", "scrypt"} {
				configCommands = append(configCommands, Command{prefix + " algorithm-type " + algorithm + " secret <SECRET...>", "Hash the secret using the selected algorithm", false})
			}
		}
	}
	for _, prefix := range []string{"username <NAME>", "username <NAME> privilege <0-15>"} {
		configCommands = append(configCommands, Command{prefix + " password 7 <SECRET...>", "Import a type 7 password", false})
	}
}
