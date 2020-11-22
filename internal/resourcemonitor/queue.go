package resourcemonitor

type queue []*monitorContext

func (m *queue) Len() int {
	return len(*m)
}

func (m *queue) Less(i, j int) bool {
	return (*m)[i].monitorEndTime.Unix() < (*m)[j].monitorEndTime.Unix()
}

func (m *queue) Swap(i, j int) {
	temp := (*m)[i]
	(*m)[i] = (*m)[j]
	(*m)[j] = temp
}

func (m *queue) Push(x interface{}) {
	*m = append(*m, x.(*monitorContext))
}

func (m *queue) Pop() interface{} {
	ret := (*m)[(*m).Len()-1]
	*m = (*m)[:(*m).Len()-1]
	return ret
}
