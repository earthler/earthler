package machines

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/earthler/earthler/core/journal"
	"github.com/earthler/earthler/core/scribe"
)

// Follower keeps this machine's copy of another machine's journal level with
// it: it asks for what comes after the copy, takes each new entry as it is
// written, and when the other machine is away, waits and asks again.
type Follower struct {
	Peer   uint64           // the machine followed
	Peers  *Peers           // where it is
	Token  string           // what this machine shows it
	Client *http.Client     // a client that trusts only the community root
	Copy   *journal.Journal // this machine's copy of its journal
	Scribe *scribe.Scribe   // nil to write in the copy alone, before the system runs
	Logf   func(format string, args ...any)

	// Back asks the other machine for the copy it keeps of this machine's
	// own journal, Self, which is then what Copy is.
	Back bool
	Self uint64

	away bool // said that the machine is away, and not yet that it is back
}

// CatchUp brings the copy level with what the other machine has written so
// far, and returns. A machine that has just joined does it before it starts,
// so it knows its community. It writes in the copy alone: the indexes take
// it when the system starts.
func (f *Follower) CatchUp(ctx context.Context) (int, error) {
	took, err := f.once(ctx, false)
	if errors.Is(err, io.EOF) {
		err = nil
	}
	return took, err
}

// receiveBatch is how many entries are handed to the scribe together at most.
const receiveBatch = 500

// FirstWait and MaxWait are how long a follower waits before it asks again
// for a machine that is away: a second, then twice as long each time, up to
// ten seconds, so a machine that comes back is followed again soon.
var FirstWait, MaxWait = time.Second, 10 * time.Second

// Run follows until ctx ends. A machine that is away is normal: the follower
// says so once, waits a little longer each time, and says when the machine
// is back.
func (f *Follower) Run(ctx context.Context) {
	wait := FirstWait
	for ctx.Err() == nil {
		got, err := f.once(ctx, true)
		if ctx.Err() != nil {
			return
		}
		if got > 0 {
			wait = FirstWait
		}
		if err != nil && !f.away {
			if f.Logf != nil {
				f.Logf("machine %d cannot be followed now: %v", f.Peer, err)
			}
			f.away = true
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = min(2*wait, MaxWait)
	}
}

// Silence is how long a stream may say nothing, not even the empty line the
// other machine sends every twenty seconds, before it is taken as dead.
var Silence = 3 * streamBeat

// once opens one stream and takes from it until it ends: when the other
// machine stops, or, when live is false, once it has sent what it has. It
// returns how many entries it took.
func (f *Follower) once(ctx context.Context, live bool) (int, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	quiet := time.AfterFunc(Silence, cancel)
	defer quiet.Stop()
	address, ok := f.Peers.Get(f.Peer)
	if !ok {
		return 0, errors.New("its address is not known")
	}
	last, _ := f.Copy.Last()
	path := "/machines/journal?after=" + strconv.FormatUint(last, 10)
	if f.Back {
		path = "/machines/copy?machine=" + strconv.FormatUint(f.Self, 10) + "&after=" + strconv.FormatUint(last, 10)
	} else if !live {
		path += "&live=0"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSuffix(address, "/")+path, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+f.Token)
	res, err := f.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return 0, fmt.Errorf("it answered %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	if got := res.Header.Get("Earthler-Machine"); got != strconv.FormatUint(f.Peer, 10) {
		return 0, fmt.Errorf("the machine at %s says it is machine %q", address, got)
	}
	if f.away && f.Logf != nil {
		f.Logf("machine %d is back", f.Peer)
	}
	f.away = false

	rd := bufio.NewReaderSize(res.Body, 64<<10)
	var batch []journal.Entry
	took := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		var err error
		if f.Scribe != nil {
			err = f.Scribe.Receive(ctx, f.Peer, batch)
		} else {
			err = f.Copy.Receive(batch)
		}
		if err == nil {
			took += len(batch)
		}
		batch = batch[:0]
		if errors.Is(err, journal.ErrNotNext) {
			err = fmt.Errorf("its journal is not the one this machine holds, which happens when it was restored from an older copy; following it stops until a person looks: %w", err)
		}
		return err
	}
	for {
		line, err := readLine(rd, journal.MaxContent+512)
		quiet.Reset(Silence)
		if err != nil {
			if ferr := flush(); ferr != nil {
				return took, ferr
			}
			return took, err
		}
		if len(line) > 0 {
			e, err := journal.ParseLine(line)
			if err != nil {
				flush()
				return took, fmt.Errorf("it sent a line that is not an entry: %w", err)
			}
			batch = append(batch, e)
		}
		// Hand over what came together, without waiting for more.
		if len(batch) >= receiveBatch || rd.Buffered() == 0 {
			if err := flush(); err != nil {
				return took, err
			}
		}
	}
}

// readLine reads one line, without its end, and refuses a line longer than
// max: a machine that sends one is not sending entries.
func readLine(rd *bufio.Reader, max int) ([]byte, error) {
	var line []byte
	for {
		part, err := rd.ReadSlice('\n')
		if len(line)+len(part) > max+1 {
			return nil, errors.New("a line longer than any entry")
		}
		line = append(line, part...)
		switch {
		case err == nil:
			return bytes.TrimSuffix(line, []byte{'\n'}), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		default:
			return nil, err
		}
	}
}
