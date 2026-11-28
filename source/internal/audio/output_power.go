package audio

// Pause the Android output stream itself, not only the source player. An open
// Oboe stream otherwise continues requesting silent audio callbacks while idle.
type audioOutput interface {
	Suspend() error
	Resume() error
}

type outputPower struct {
	output  audioOutput
	running bool
}

func (p *outputPower) set(running bool) error {
	if p.output == nil || running == p.running {
		return nil
	}
	var err error
	if running {
		err = p.output.Resume()
	} else {
		err = p.output.Suspend()
	}
	if err == nil {
		p.running = running
	}
	return err
}
