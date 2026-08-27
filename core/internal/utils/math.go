package utils

import (
	"math"
)

// Min returns the minimum of two integers
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max returns the maximum of two integers
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// MinFloat returns the minimum of two floats
func MinFloat(a, b float64) float64 {
	return math.Min(a, b)
}

// MaxFloat returns the maximum of two floats
func MaxFloat(a, b float64) float64 {
	return math.Max(a, b)
}

// Abs returns the absolute value of an integer
func Abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// AbsFloat returns the absolute value of a float
func AbsFloat(x float64) float64 {
	return math.Abs(x)
}

// Round rounds a float to specified decimal places
func Round(x float64, places int) float64 {
	shift := math.Pow(10, float64(places))
	return math.Round(x*shift) / shift
}

// Ceil rounds up to the nearest integer
func Ceil(x float64) int {
	return int(math.Ceil(x))
}

// Floor rounds down to the nearest integer
func Floor(x float64) int {
	return int(math.Floor(x))
}

// Clamp clamps a value between min and max
func Clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// ClampFloat clamps a float value between min and max
func ClampFloat(value, min, max float64) float64 {
	return math.Max(min, math.Min(max, value))
}

// Percentage calculates percentage
func Percentage(value, total float64) float64 {
	if total == 0 {
		return 0
	}
	return (value / total) * 100
}

// PercentageInt calculates percentage for integers
func PercentageInt(value, total int) float64 {
	if total == 0 {
		return 0
	}
	return (float64(value) / float64(total)) * 100
}

// Sum calculates sum of integers
func Sum(values ...int) int {
	total := 0
	for _, v := range values {
		total += v
	}
	return total
}

// SumFloat calculates sum of floats
func SumFloat(values ...float64) float64 {
	total := 0.0
	for _, v := range values {
		total += v
	}
	return total
}

// Average calculates average of integers
func Average(values ...int) float64 {
	if len(values) == 0 {
		return 0
	}
	return float64(Sum(values...)) / float64(len(values))
}

// AverageFloat calculates average of floats
func AverageFloat(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return SumFloat(values...) / float64(len(values))
}

// Median calculates median of integers
func Median(values ...int) float64 {
	if len(values) == 0 {
		return 0
	}

	// Sort values
	sorted := make([]int, len(values))
	copy(sorted, values)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i] > sorted[j] {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	middle := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return float64(sorted[middle-1]+sorted[middle]) / 2
	}
	return float64(sorted[middle])
}

// StandardDeviation calculates standard deviation
func StandardDeviation(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}

	mean := AverageFloat(values...)
	variance := 0.0
	for _, v := range values {
		variance += (v - mean) * (v - mean)
	}
	variance /= float64(len(values))
	return math.Sqrt(variance)
}

// IsPowerOfTwo checks if a number is a power of two
func IsPowerOfTwo(n int) bool {
	return n > 0 && (n&(n-1)) == 0
}

// NextPowerOfTwo returns the next power of two
func NextPowerOfTwo(n int) int {
	if n <= 0 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	return n + 1
}

// GCD calculates greatest common divisor
func GCD(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// LCM calculates least common multiple
func LCM(a, b int) int {
	return a * b / GCD(a, b)
}

// Factorial calculates factorial
func Factorial(n int) int {
	if n <= 1 {
		return 1
	}
	return n * Factorial(n-1)
}

// Fibonacci calculates fibonacci number
func Fibonacci(n int) int {
	if n <= 1 {
		return n
	}
	a, b := 0, 1
	for i := 2; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}

// IsPrime checks if a number is prime
func IsPrime(n int) bool {
	if n <= 1 {
		return false
	}
	if n <= 3 {
		return true
	}
	if n%2 == 0 || n%3 == 0 {
		return false
	}
	for i := 5; i*i <= n; i += 6 {
		if n%i == 0 || n%(i+2) == 0 {
			return false
		}
	}
	return true
}
