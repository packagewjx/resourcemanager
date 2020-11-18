package monitor

type monitoringQueue []*monitorContext

func (m *monitoringQueue) Len() int {
	return len(*m)
}

func (m *monitoringQueue) Less(i, j int) bool {
	return (*m)[i].monitorEndTime.Unix() < (*m)[j].monitorEndTime.Unix()
}

func (m *monitoringQueue) Swap(i, j int) {
	temp := (*m)[i]
	(*m)[i] = (*m)[j]
	(*m)[j] = temp
}

func (m *monitoringQueue) Push(x interface{}) {
	*m = append(*m, x.(*monitorContext))
}

func (m *monitoringQueue) Pop() interface{} {
	ret := (*m)[(*m).Len()-1]
	*m = (*m)[:(*m).Len()-1]
	return ret
}
