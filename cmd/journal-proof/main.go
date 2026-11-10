// Command journal-proof checks the journal against a real power cut.
//
// It needs two machines. One stays on and remembers what the other one
// confirmed. The other one writes without stopping until someone pulls its
// plug. After the power is back, every entry that was confirmed has to be
// in the journal. See README.md in this folder.
package main

import (
	"flag"
	"fmt"
	"os"
)

const usage = `journal-proof checks the journal against a real power cut.

On a machine that stays on:
  journal-proof collect [-listen :7070] [-file journal-proof.log]

On the machine under test, before each power cut:
  journal-proof write -collector HOST:7070 [-data journal-proof-data]

On the machine under test, to check without writing:
  journal-proof check -collector HOST:7070 [-data journal-proof-data]
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	flags := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	listen := flags.String("listen", ":7070", "address the collector listens on")
	file := flags.String("file", "journal-proof.log", "where the collector keeps what it heard")
	addr := flags.String("collector", "", "address of the collector, as host:port")
	data := flags.String("data", "journal-proof-data", "folder of the journal under test")
	flags.Parse(os.Args[2:])

	var err error
	switch os.Args[1] {
	case "collect":
		err = collect(*listen, *file)
	case "write":
		err = needCollector(*addr, func() error { return write(*data, *addr) })
	case "check":
		err = needCollector(*addr, func() error { return check(*data, *addr) })
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "journal-proof:", err)
		os.Exit(1)
	}
}

func needCollector(addr string, run func() error) error {
	if addr == "" {
		return fmt.Errorf("pass -collector with the address of the machine that stays on")
	}
	return run()
}
