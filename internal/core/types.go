package core

type ProgramMetric struct {
	Id           string
	MRC          []float32
	Instructions int
	L1Hit        int
	L2Hit        int
	L3Hit        int
	L3Miss       int
}

func (p ProgramMetric) Api() float32 {
	return float32(p.L1Hit+p.L2Hit+p.L3Hit+p.L3Miss) / float32(p.Instructions)
}

type CLOSScheme struct {
	CLOSNum       int
	WayBit        int
	MemThrottle   int
	ProcessGroups []*ProcessGroup
}

type CLOSCapabilityInfo struct {
	NumCatClos uint
	NumMbaClos uint
	MaxLLCWays uint
	MinLLCWays uint
}

type ProcessGroup struct {
	Id  string
	Pid []int
}
