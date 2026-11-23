package core

import (
	"dcms/internal/store"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode"
)

var trMap = map[rune]string{'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "yo", 'ж': "zh", 'з': "z", 'и': "i", 'й': "j",
	'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f", 'х': "h", 'ц': "c",
	'ч': "ch", 'ш': "sh", 'щ': "shh", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya", 'і': "i", 'ї': "yi", 'є': "ye", 'ґ': "g"}

// Translit — транслитерация кириллицы в латиницу (для имён файлов).
func Translit(s string) string {
	var sb strings.Builder
	for _, r := range s {
		lr := unicode.ToLower(r)
		if t, ok := trMap[lr]; ok {
			if lr != r && t != "" {
				t = strings.ToUpper(t[:1]) + t[1:]
			}
			sb.WriteString(t)
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// SafeName — безопасное имя файла: транслит, только [A-Za-z0-9._-].
func SafeName(s string) string {
	s = Translit(s)
	var sb strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-', r == '_':
			sb.WriteRune(r)
		case r == ' ':
			sb.WriteByte('_')
		}
	}
	out := strings.Trim(sb.String(), ".")
	if out == "" {
		out = "file"
	}
	return out
}

// SizeStr — размер файла для людей (как size_file.php).
func SizeStr(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.2f ГБ", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.2f МБ", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.2f КБ", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d байт", n)
}

// Plural — склонение по числу: Plural(5, "сообщение","сообщения","сообщений").
func Plural(n int64, one, few, many string) string {
	n = n % 100
	if n >= 11 && n <= 19 {
		return many
	}
	switch n % 10 {
	case 1:
		return one
	case 2, 3, 4:
		return few
	}
	return many
}

// Ext — расширение файла в нижнем регистре без точки.
func Ext(name string) string {
	i := strings.LastIndex(name, ".")
	if i < 0 {
		return ""
	}
	return strings.ToLower(name[i+1:])
}

// RuneLen — длина строки в символах.
func RuneLen(s string) int { return len([]rune(s)) }

func baseID(id int64) (b storeBase) { b.ID = id; return }

type storeBase = store.Base

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
