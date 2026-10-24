package cli

func init() {
	for _, kind := range []string{"scrub", "balance"} {
		execCommands = append(execCommands, Command{"show storage datastore <NAME> " + kind, "Datastore " + kind + " status", false})
		for _, action := range []string{"start", "cancel"} {
			execCommands = append(execCommands, Command{"request storage datastore <NAME> " + kind + " " + action, "Datastore " + kind + " " + action, false})
		}
	}
	execCommands = append(execCommands, Command{"show storage datastore <NAME> snapshots", "Datastore snapshots", false}, Command{"request storage datastore <NAME> snapshot create <NAME>", "Create a read-only filesystem snapshot", false}, Command{"request storage datastore <NAME> snapshot delete <NAME> confirm <NAME>", "Delete a named snapshot", false})
}
