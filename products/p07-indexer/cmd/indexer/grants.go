package main

import "github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/service"

type grantFile[T any] string

func (f grantFile[T]) read() (map[string]T, error) {
	return service.ClientGrantFile[T](f).Read()
}

func (f grantFile[T]) lookup(key string) (T, bool) {
	return service.ClientGrantFile[T](f).Lookup(key)
}
