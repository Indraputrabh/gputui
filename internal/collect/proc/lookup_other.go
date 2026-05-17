//go:build !linux

package proc

func lookup(_ int) Info {
	return Info{User: "?", Cmd: "?", RSSKB: 0}
}

func lookupCPU(_ int) CPUSample {
	return CPUSample{}
}
