package appliance

// Cancellation is a protocol event, never a magic filename or password.
func (s *loginSession) cancelDialog() reply {
	s.pendingCopy = nil
	s.pendingReload = nil
	s.pendingFile = nil
	r := reply{Prompt: s.cli.Prompt()}
	if s.setup != nil {
		s.setup = nil
		r.Output = "Setup cancelled. No configuration saved."
	}
	return r
}
