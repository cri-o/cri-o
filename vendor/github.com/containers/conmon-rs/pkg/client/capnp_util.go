package client

import (
	"fmt"

	"capnproto.org/go/capnp/v3"
	"github.com/containers/conmon-rs/internal/proto"
)

func stringSliceToTextList(
	data []string,
	newFunc func(int32) (capnp.TextList, error),
) error {
	l := int32(len(data))
	if l == 0 {
		return nil
	}

	list, err := newFunc(l)
	if err != nil {
		return fmt.Errorf("create list: %w", err)
	}

	for i, item := range data {
		if err := list.Set(i, item); err != nil {
			return fmt.Errorf("set list element: %w", err)
		}
	}

	return nil
}

func stringStringMapToMapEntryList(
	data map[string]string,
	newFunc func(int32) (proto.Conmon_TextTextMapEntry_List, error),
) error {
	l := int32(len(data))
	if l == 0 {
		return nil
	}

	list, err := newFunc(l)
	if err != nil {
		return fmt.Errorf("create map: %w", err)
	}

	i := 0
	for key, value := range data {
		entry := list.At(i)
		if err := entry.SetKey(key); err != nil {
			return fmt.Errorf("set map key: %w", err)
		}
		if err := entry.SetValue(value); err != nil {
			return fmt.Errorf("set map value: %w", err)
		}
		i++
	}

	return nil
}

func remoteFDSliceToUInt64List(src []RemoteFD, newFunc func(int32) (capnp.UInt64List, error)) error {
	l := int32(len(src))
	if l == 0 {
		return nil
	}
	list, err := newFunc(l)
	if err != nil {
		return err
	}
	for i := range src {
		list.Set(i, uint64(src[i]))
	}

	return nil
}
