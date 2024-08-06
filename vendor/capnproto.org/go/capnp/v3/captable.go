package capnp

// CapTable is the indexed list of the clients referenced in the
// message. Capability pointers inside the message will use this
// table to map pointers to Clients.   The table is populated by
// the RPC system.
//
// https://capnproto.org/encoding.html#capabilities-interfaces
type CapTable struct {
	cs []Client
}

// Reset the cap table, releasing all capabilities and setting
// the length to zero.   Clients passed as arguments are added
// to the table after zeroing, such that ct.Len() == len(cs).
func (ct *CapTable) Reset(cs ...Client) {
	for _, c := range ct.cs {
		c.Release()
	}

	ct.cs = append(ct.cs[:0], cs...)
}

// Len returns the number of capabilities in the table.
func (ct CapTable) Len() int {
	return len(ct.cs)
}

// At returns the capability at the given index of the table.
func (ct CapTable) At(i int) Client {
	return ct.cs[i]
}

// Contains returns true if the supplied interface corresponds
// to a client already present in the table.
func (ct CapTable) Contains(ifc Interface) bool {
	return ifc.IsValid() && ifc.Capability() < CapabilityID(ct.Len())
}

// Get the client corresponding to the supplied interface.  It
// returns a null client if the interface's CapabilityID isn't
// in the table.
func (ct CapTable) Get(ifc Interface) (c Client) {
	if ct.Contains(ifc) {
		c = ct.cs[ifc.Capability()]
	}

	return
}

// Set the client for the supplied capability ID.  If a client
// for the given ID already exists, it will be replaced without
// releasing.
func (ct CapTable) Set(id CapabilityID, c Client) {
	ct.cs[id] = c
}

// Add appends a capability to the message's capability table and
// returns its ID.  It "steals" c's reference: the Message will release
// the client when calling Reset.
func (ct *CapTable) Add(c Client) CapabilityID {
	ct.cs = append(ct.cs, c)
	return CapabilityID(ct.Len() - 1)
}
