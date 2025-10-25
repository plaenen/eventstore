package validators

import (
	"fmt"
	"strings"
)

func MaskString(value string) string {
	if len(value) < 4 {
		return "************"
	}
	maskLength := len(value) - 4
	mask := strings.Repeat("*", maskLength)
	return fmt.Sprintf("%s%s", mask, value[len(value)-4:])
}

func MaskPassword(value string) string {
	return "*************************"
}
