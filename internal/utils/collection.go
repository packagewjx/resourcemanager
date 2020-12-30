package utils

func IntListDifferent(old, new []int) bool {
	if len(old) != len(new) {
		return true
	}
	oldMap := make(map[int]struct{})
	for _, i := range old {
		oldMap[i] = struct{}{}
	}
	for _, i := range new {
		if _, ok := oldMap[i]; !ok {
			return true
		}
	}
	return false
}
