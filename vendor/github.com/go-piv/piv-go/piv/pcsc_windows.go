// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package piv

import (
	"fmt"
	"syscall"
	"unsafe"
)

var (
	winscard                  = syscall.NewLazyDLL("Winscard.dll")
	procSCardEstablishContext = winscard.NewProc("SCardEstablishContext")
	procSCardListReadersW     = winscard.NewProc("SCardListReadersW")
	procSCardReleaseContext   = winscard.NewProc("SCardReleaseContext")
	procSCardConnectW         = winscard.NewProc("SCardConnectW")
	procSCardDisconnect       = winscard.NewProc("SCardDisconnect")
	procSCardBeginTransaction = winscard.NewProc("SCardBeginTransaction")
	procSCardEndTransaction   = winscard.NewProc("SCardEndTransaction")
	procSCardTransmit         = winscard.NewProc("SCardTransmit")
)

const (
	scardScopeSystem      = 2
	scardShareExclusive   = 1
	scardLeaveCard        = 0
	scardProtocolT1       = 2
	scardPCIT1            = 0
	maxBufferSizeExtended = (4 + 3 + (1 << 16) + 3 + 2)
	rcSuccess             = 0
)

func scCheck(rc uintptr) error {
	if rc == rcSuccess {
		return nil
	}
	return &scErr{int64(rc)}
}

func isRCNoReaders(rc uintptr) bool {
	return rc == 0x8010002E
}

type scContext struct {
	ctx syscall.Handle
}

func newSCContext() (*scContext, error) {
	var ctx syscall.Handle

	r0, _, _ := procSCardEstablishContext.Call(
		uintptr(scardScopeSystem),
		uintptr(0),
		uintptr(0),
		uintptr(unsafe.Pointer(&ctx)),
	)
	if err := scCheck(r0); err != nil {
		return nil, err
	}
	return &scContext{ctx: ctx}, nil
}

func (c *scContext) Close() error {
	r0, _, _ := procSCardReleaseContext.Call(uintptr(c.ctx))
	return scCheck(r0)
}

func (c *scContext) ListReaders() ([]string, error) {
	var n uint32
	r0, _, _ := procSCardListReadersW.Call(
		uintptr(c.ctx),
		uintptr(unsafe.Pointer(nil)),
		uintptr(unsafe.Pointer(nil)),
		uintptr(unsafe.Pointer(&n)),
	)

	if isRCNoReaders(r0) {
		return nil, nil
	}

	if err := scCheck(r0); err != nil {
		return nil, err
	}

	d := make([]uint16, n)
	r0, _, _ = procSCardListReadersW.Call(
		uintptr(c.ctx),
		uintptr(unsafe.Pointer(nil)),
		uintptr(unsafe.Pointer(&d[0])),
		uintptr(unsafe.Pointer(&n)),
	)
	if err := scCheck(r0); err != nil {
		return nil, err
	}

	var readers []string
	j := 0
	for i := 0; i < len(d); i++ {
		if d[i] != 0 {
			continue
		}
		readers = append(readers, syscall.UTF16ToString(d[j:i]))
		j = i + 1

		if d[i+1] == 0 {
			break
		}
	}

	return readers, nil
}

func (c *scContext) Connect(reader string) (*scHandle, error) {
	var (
		handle         syscall.Handle
		activeProtocol uint16
	)
	r0, _, _ := procSCardConnectW.Call(
		uintptr(c.ctx),
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(reader))),
		scardShareExclusive,
		scardProtocolT1,
		uintptr(unsafe.Pointer(&handle)),
		uintptr(activeProtocol),
	)
	if err := scCheck(r0); err != nil {
		return nil, err
	}
	return &scHandle{handle}, nil
}

type scHandle struct {
	handle syscall.Handle
}

func (h *scHandle) Close() error {
	r0, _, _ := procSCardDisconnect.Call(uintptr(h.handle), scardLeaveCard)
	return scCheck(r0)
}

func (h *scHandle) Begin() (*scTx, error) {
	r0, _, _ := procSCardBeginTransaction.Call(uintptr(h.handle))
	if err := scCheck(r0); err != nil {
		return nil, err
	}
	return &scTx{h.handle}, nil
}

func (t *scTx) Close() error {
	r0, _, _ := procSCardEndTransaction.Call(uintptr(t.handle), scardLeaveCard)
	return scCheck(r0)
}

type scTx struct {
	handle syscall.Handle
}

func (t *scTx) transmit(req []byte) (more bool, b []byte, err error) {
	var resp [maxBufferSizeExtended]byte
	reqN := len(req)
	respN := len(resp)
	r0, _, _ := procSCardTransmit.Call(
		uintptr(t.handle),
		uintptr(scardPCIT1),
		uintptr(unsafe.Pointer(&req[0])),
		uintptr(reqN),
		uintptr(0),
		uintptr(unsafe.Pointer(&resp[0])),
		uintptr(unsafe.Pointer(&respN)),
	)

	if err := scCheck(r0); err != nil {
		return false, nil, fmt.Errorf("transmitting request: %w", err)
	}
	if respN < 2 {
		return false, nil, fmt.Errorf("scard response too short: %d", respN)
	}
	sw1 := resp[respN-2]
	sw2 := resp[respN-1]
	if sw1 == 0x90 && sw2 == 0x00 {
		return false, resp[:respN-2], nil
	}
	if sw1 == 0x61 {
		return true, resp[:respN-2], nil
	}
	return false, nil, &apduErr{sw1, sw2}
}
