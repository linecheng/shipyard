package utils

import (
	"errors"
)

func Errors(err ...error) error {
	var res = errors.New("")
	for _, item := range err {
		res = _errors(res.Error()+"", item)
	}

	return res
}

func _errors(s string, err error) error {
	return errors.New(s + "  ->  " + err.Error())
}
