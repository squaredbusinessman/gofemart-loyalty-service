package service

// Фиксируем что строка это набор символов от "0" до "9"
func isDigits(number string) bool {
	if number == "" {
		return false
	}

	for i := 0; i < len(number); i++ {
		if number[i] < '0' || number[i] > '9' {
			return false
		}
	}
	return true
}

// Реализация алгоритма Луна
func isValidLuhn(number string) bool {
	if number == "" {
		return false
	}

	sum := 0
	double := false

	for i := len(number) - 1; i >= 0; i-- {
		d := int(number[i] - '0')

		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}

		sum += d
		double = !double
	}

	return sum%10 == 0
}
