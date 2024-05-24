package postfixnotation

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// Переводит инфиксную запись в постфиксную.
// При успехе возвращает массив с постфиксной записью и nil.
// При неудаче возвращает nil и ошибку.
func ToPostfixNotation(expression string) ([]any, error) {
	expression = strings.ReplaceAll(expression, " ", "") //убираем пробелы
	rexp := []rune(expression)
	//дальше одновременно идёт и проверка выражения на корректность, и его запись в постфикс. н.
	brackets := 0                    //скобки; ( - +1; ) - -1
	point := false                   //две точки не могут идти в одном числе, так?
	integer := 0                     //целая часть
	fraction := 0                    //дробная часть
	stack := make([]any, 0)          //для конвертации
	queue := make([]any, 0)          //для конвертации
	for i := 0; i < len(rexp); i++ { //перебираем выражение
		r := rexp[i]
		if i != 0 {
			//тут проверяем, надо ли число кинуть в queue
			if unicode.IsDigit(rexp[i-1]) && !unicode.IsDigit(r) && r != '.' {
				s := fmt.Sprint(integer) + "." + fmt.Sprint(fraction)
				n, err := strconv.ParseFloat(s, 64) //переводим число в float64
				if err != nil {
					return nil, err
				}
				queue = append(queue, n)
				point = false
				integer = 0
				fraction = 0
			}
			//проверка на два подряд идущих знака (так низя)
			if strings.Contains("/*-+", string(r)) && strings.Contains("/*-+", string(rexp[i-1])) {
				return nil, errors.New("invalid expression: two signs in a row")
			}
		}
		switch {
		case unicode.IsDigit(r): //цифра
			n, _ := strconv.Atoi(string(r))
			if point {
				fraction *= 10
				fraction += n
			} else {
				integer *= 10
				integer += n
			}
			if i == len(rexp)-1 {
				s := fmt.Sprint(integer) + "." + fmt.Sprint(fraction)
				n, err := strconv.ParseFloat(s, 64) //переводим число в float64
				if err != nil {
					return nil, err
				}
				queue = append(queue, n)
			}
		case r == '.': //точка
			if point {
				return nil, errors.New("invalid expression: two points in one number")
			}
			point = true
		case r == '*' || r == '/': //операторы с высоким приоритетом
			if len(stack) != 0 {
				if stack[len(stack)-1] == "*" || stack[len(stack)-1] == "/" {
					for len(stack) != 0 {
						if stack[len(stack)-1] != "+" && stack[len(stack)-1] != "-" &&
							stack[len(stack)-1] != "(" {
							queue = append(queue, stack[len(stack)-1]) //выгружаем в очередь
							stack = stack[:len(stack)-1]               //убираем последний элемент стека
						} else {
							if stack[len(stack)-1] == "(" {
								stack = stack[:len(stack)-1]
							}
							break
						}
					}
				}
			}
			stack = append(stack, string(r))
		case r == '+' || r == '-': //операторы с низким приоритетом
			if (i == 0 || rexp[i-1] == '(') && r == '-' {
				//унарный минус - просто добавим умножение на -1
				queue = append(queue, float64(-1))
				rexp = append(rexp[:i+1], append([]rune{'*'}, rexp[i+1:]...)...)
				rexp[i] = ' ' //чтоб проверка на два знака не ругалась
			} else if len(stack) == 0 {
				stack = append(stack, string(r))
			} else if stack[len(stack)-1] == "(" {
				stack = append(stack, string(r))
			} else if stack[len(stack)-1] == "*" || stack[len(stack)-1] == "/" {
				for len(stack) != 0 {
					if stack[len(stack)-1] != "(" {
						queue = append(queue, stack[len(stack)-1]) //выгружаем в очередь
						stack = stack[:len(stack)-1]               //убираем последний элемент стека
					} else {
						stack = stack[:len(stack)-1]
						break
					}
				}
				stack = append(stack, string(r))
			} else {
				queue = append(queue, stack[len(stack)-1])
				stack[len(stack)-1] = string(r)
			}
		case r == '(': //левая скобка
			brackets++
			stack = append(stack, string(r))
		case r == ')': //правая скобка
			brackets--
			if brackets < 0 {
				return nil, errors.New("invalid expression: brackets mismatch")
			}
			for len(stack) != 0 {
				if stack[len(stack)-1] != "(" {
					queue = append(queue, stack[len(stack)-1]) //выгружаем в очередь
					stack = stack[:len(stack)-1]               //убираем последний элемент стека
				} else {
					stack = stack[:len(stack)-1]
					break
				}
			}
		default:
			return nil, errors.New("invalid expression")
		}

	}
	if brackets != 0 {
		return nil, errors.New("invalid expression: brackets mismatch")
	}
	for len(stack) != 0 {
		queue = append(queue, stack[len(stack)-1]) //выгружаем в очередь
		stack = stack[:len(stack)-1]
	}
	//проверим, что чисел и знаков нормально
	nums := 0
	for _, v := range queue {
		if _, ok := v.(float64); ok {
			nums++
		}
	}
	signs := len(queue) - nums
	if nums != signs+1 {
		return nil, errors.New("invalid expression: operators mismatch")
	}
	return queue, nil
}
