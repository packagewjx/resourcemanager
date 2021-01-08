package core

type ProcessGroup struct {
	Id  string
	Pid []int
}

func (p *ProcessGroup) Clone() Cloneable {
	cpid := make([]int, len(p.Pid))
	copy(cpid, p.Pid)
	return &ProcessGroup{
		Id:  p.Id,
		Pid: cpid,
	}
}

type Cloneable interface {
	Clone() Cloneable
}
