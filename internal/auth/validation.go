package auth

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.]{1,32}$`)

var reservedUsernames = map[string]bool{
	"admin": true, "administrator": true, "moderator": true, "support": true, "root": true,
	"system": true, "settings": true, "login": true, "logout": true, "register": true,
	"api": true, "polka": true, "shelf": true, "null": true, "undefined": true,
}

func validEmail(value string) bool {
	if utf8.RuneCountInString(value) > 254 || strings.Count(value, "@") != 1 || strings.IndexFunc(value, unicode.IsSpace) >= 0 || strings.ContainsRune(value, 0) {
		return false
	}
	local, domain, _ := strings.Cut(value, "@")
	if local == "" || domain == "" {
		return false
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

func usernameError(value string, registration bool) string {
	if value == "" {
		return "Введите имя пользователя"
	}
	if !usernamePattern.MatchString(value) {
		return "Имя пользователя должно содержать от 1 до 32 символов: латинские буквы, цифры, `_` или `.`"
	}
	if strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") || strings.Contains(value, "..") {
		return "Имя пользователя не может начинаться или заканчиваться точкой и содержать две точки подряд"
	}
	if registration && reservedUsernames[strings.ToLower(value)] {
		return "Это имя пользователя недоступно"
	}
	return ""
}

func passwordError(value string, registration bool) string {
	if value == "" {
		return "Введите пароль"
	}
	length := utf8.RuneCountInString(value)
	if registration && (length < 8 || length > 32) {
		return "Пароль должен содержать от 8 до 32 символов"
	}
	if !registration && length > 32 {
		return "Пароль не должен превышать 32 символа"
	}
	if !utf8.ValidString(value) {
		return "Пароль содержит недопустимые символы"
	}
	for _, r := range value {
		if !unicode.IsGraphic(r) {
			return "Пароль содержит недопустимые символы"
		}
	}
	return ""
}

func registrationErrors(email, username, password, confirmation string) map[string]string {
	fields := make(map[string]string)
	if email == "" {
		fields["email"] = "Введите адрес электронной почты"
	} else if !validEmail(email) {
		fields["email"] = "Введите корректный адрес электронной почты"
	}
	if message := usernameError(username, true); message != "" {
		fields["username"] = message
	}
	if message := passwordError(password, true); message != "" {
		fields["password"] = message
	}
	if confirmation == "" {
		fields["password_confirmation"] = "Подтвердите пароль"
	} else if confirmation != password {
		fields["password_confirmation"] = "Пароли не совпадают"
	}
	return fields
}

func loginErrors(identifier, password string) map[string]string {
	fields := make(map[string]string)
	if identifier == "" {
		fields["identifier"] = "Введите адрес электронной почты или имя пользователя"
	} else {
		valid := usernameError(identifier, false) == ""
		if strings.Contains(identifier, "@") {
			valid = validEmail(identifier)
		}
		if !valid {
			fields["identifier"] = "Введите корректный адрес электронной почты или имя пользователя"
		}
	}
	if message := passwordError(password, false); message != "" {
		fields["password"] = message
	}
	return fields
}
