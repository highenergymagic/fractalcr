package config

import "fmt"

// A nil line policy inherits the appliance's IOS terminal defaults. Keeping
// the complete policy together makes zero length/size distinct from omission.
type LineTerminal struct {
	Length      int  `json:"length"`
	HistorySize int  `json:"history_size"`
	History     bool `json:"history"`
	Editing     bool `json:"editing"`
}

func DefaultLineTerminal() LineTerminal {
	return LineTerminal{Length: 24, HistorySize: 10, History: true, Editing: true}
}

func (p *LineTerminal) Effective() LineTerminal {
	if p == nil {
		return DefaultLineTerminal()
	}
	return *p
}

func (p *LineTerminal) Validate() error {
	t := p.Effective()
	if t.Length < 0 || t.Length > 512 || t.HistorySize < 0 || t.HistorySize > 256 {
		return fmt.Errorf("invalid line terminal length or history size")
	}
	return nil
}

func (a AAA) Terminal(number int) LineTerminal {
	if number == 0 {
		return a.ConsoleTerminal.Effective()
	}
	return a.Line(number - 1).Terminal.Effective()
}

func (a VTYLine) Equal(b VTYLine) bool {
	return a.Timeout == b.Timeout && a.SSH == b.SSH && a.Terminal.Effective() == b.Terminal.Effective()
}
