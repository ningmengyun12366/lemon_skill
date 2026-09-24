package apikey

import "errors"

const InvalidFormatMessage = "apikey格式不正确"

func ValidateFormat(value string) error {
	if value == "" {
		return errors.New(InvalidFormatMessage)
	}
	for _, char := range value {
		if char >= '0' && char <= '9' {
			continue
		}
		if char >= 'A' && char <= 'Z' {
			continue
		}
		if char >= 'a' && char <= 'z' {
			continue
		}
		if char == '_' {
			continue
		}
		return errors.New(InvalidFormatMessage)
	}
	return nil
}
