package idgen

import (
	"fmt"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

type nanoidOptions struct {
	prefix   string
	length   int
	alphabet string
}

type Option func(*nanoidOptions)

func WithPrefix(prefix string) Option {
	return func(o *nanoidOptions) {
		o.prefix = prefix
	}
}

func WithLength(length int) Option {
	return func(o *nanoidOptions) {
		o.length = length
	}
}

func WithAlphabet(alphabet string) Option {
	return func(o *nanoidOptions) {
		o.alphabet = alphabet
	}
}

func NanoID(opts ...Option) (string, error) {
	options := &nanoidOptions{
		length: 21, // Default length used by go-nanoid
	}
	for _, opt := range opts {
		opt(options)
	}

	var id string
	var err error

	if options.alphabet != "" {
		id, err = gonanoid.Generate(options.alphabet, options.length)
	} else {
		id, err = gonanoid.Generate("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", options.length)
	}

	if err != nil {
		return "", err
	}

	if options.prefix != "" {
		return fmt.Sprintf("%s_%s", options.prefix, id), nil
	}
	return id, nil
}

func MustNanoID(opts ...Option) string {
	id, err := NanoID(opts...)
	if err != nil {
		panic(err)
	}
	return id
}
