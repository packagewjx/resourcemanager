package utils

func NumBits(n int) int {
	res := 0
	for n > 0 {
		res += n & 1
		n >>= 1
	}
	return res
}

func GetLowestBits(n int) int {
	return (1 << n) - 1
}
