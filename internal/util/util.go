package util

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// 定义每个大小单位
	_           = iota // ignore first value by assigning to blank identifier
	KB ByteSize = 1 << (10 * iota)
	MB
	GB
	TB
	PB
	EB
	ZB
	YB
)

type ByteSize float64

func (b ByteSize) String() string {
	switch {
	case b >= YB:
		return fmt.Sprintf("%.2fYB", b/YB)
	case b >= ZB:
		return fmt.Sprintf("%.2fZB", b/ZB)
	case b >= EB:
		return fmt.Sprintf("%.2fEB", b/EB)
	case b >= PB:
		return fmt.Sprintf("%.2fPB", b/PB)
	case b >= TB:
		return fmt.Sprintf("%.2fTB", b/TB)
	case b >= GB:
		return fmt.Sprintf("%.2fGB", b/GB)
	case b >= MB:
		return fmt.Sprintf("%.2fMB", b/MB)
	case b >= KB:
		return fmt.Sprintf("%.2fKB", b/KB)
	}
	return fmt.Sprintf("%.2fB", b)
}

func FormatBytes(bytes int64) string {
	b := ByteSize(bytes)
	return b.String()
}

// ParseByteSize 将字符串如"2KB"转换为字节大小的整数表示
func ParseByteSize(s string) (int64, error) {
	s = strings.TrimSpace(s) // 移除字符串两端的空白字符
	// 获取最后一个字符的位置，这个字符应该是字母而不是数字
	lastDigitIndex := strings.LastIndexFunc(s, func(r rune) bool { return r >= '0' && r <= '9' })
	if lastDigitIndex == -1 {
		return 0, fmt.Errorf("invalid size: %s", s)
	}

	// 提取数字部分和单位部分
	numberPart, unitPart := s[:lastDigitIndex+1], strings.ToUpper(s[lastDigitIndex+1:])
	// 解析数字部分
	number, err := strconv.ParseFloat(numberPart, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid number: %s", numberPart)
	}

	var bytes int64
	switch unitPart {
	case "B":
		bytes = int64(number)
	case "KB":
		bytes = int64(number * float64(KB))
	case "MB":
		bytes = int64(number * float64(MB))
	case "GB":
		bytes = int64(number * float64(GB))
	case "TB":
		bytes = int64(number * float64(TB))
	case "PB":
		bytes = int64(number * float64(PB))
	case "EB":
		bytes = int64(number * float64(EB))
	// 如果需要，可以继续添加更大的单位
	default:
		return 0, fmt.Errorf("unrecognized unit: %s", unitPart)
	}

	return bytes, nil
}
