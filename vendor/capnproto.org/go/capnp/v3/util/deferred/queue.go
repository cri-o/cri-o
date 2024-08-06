// Package deferred provides tools for deferring actions
// to be run at a later time. These can be thought of as
// an extension and generalization of the language's built-in
// defer statement.
package deferred

// A Queue is a queue of actions to run at some later time.
// The actions will be run in the order they are added with
// Defer. Note that this is different from the language's
// built-in defer statement, which runs actions in reverse
// order (future versions of this package may add a Stack
// type to support those semantics).
//
// The zero value is an empty queue.
//
// The advantage of this vs. built-in defer is that it needn't
// be tied to the scope of a single function; for example,
// you can write code like:
//
// q := &Queue{}
// defer q.Run()
// f(q) // pass the queue to a subroutine, which may queue
//      // up actions to be run after *this* function returns.
type Queue []func()

// Run runs all deferred actions, in the order they were added.
func (q *Queue) Run() {
	funcs := *q
	for i, f := range funcs {
		if f != nil {
			f()
			funcs[i] = nil
		}
	}
}

// Defer adds f to the list of functions to call when Run()
// is invoked.
func (q *Queue) Defer(f func()) {
	*q = append(*q, f)
}
