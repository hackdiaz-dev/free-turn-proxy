package config

import (
	"encoding/base64"
	"io"
	"reflect"
	"testing"
)

// flag останавливается на первом аргументе без дефиса, поэтому флаги после ссылки
// freeturn:// разбираются отдельно. Порядок не должен влиять на результат.
func TestParseClient_URIAndFlagsOrder(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"provider":"vk","peer":"1.2.3.4:56000"}`))
	link := "freeturn://" + payload
	const call = "https://vk.ru/call/join/abcdef"

	before, err := ParseClient([]string{"-client-id", "deadbeef", "-link", call, link}, io.Discard)
	if err != nil {
		t.Fatalf("flags before the link: %v", err)
	}
	after, err := ParseClient([]string{link, "-client-id", "deadbeef", "-link", call}, io.Discard)
	if err != nil {
		t.Fatalf("flags after the link: %v", err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("argument order changed the result:\nbefore: %+v\nafter:  %+v", before, after)
	}
}

func TestParseClient_UnknownFlagAfterURI(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"provider":"vk","peer":"1.2.3.4:56000"}`))
	_, err := ParseClient([]string{"freeturn://" + payload, "-no-such-flag"}, io.Discard)
	if err == nil {
		t.Fatal("expected an error for an unknown flag after the link")
	}
}
