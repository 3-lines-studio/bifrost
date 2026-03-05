package core

import "strconv"

func HashContent(content []byte) string {
	result := 0
	for _, b := range content {
		result = (result*31 + int(b)) % 1000000007
	}
	return strconv.Itoa(result)
}
