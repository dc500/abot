package dt

import (
	"database/sql"
	"encoding/json"

	log "github.com/avabot/ava/Godeps/_workspace/src/github.com/Sirupsen/logrus"
	"github.com/avabot/ava/Godeps/_workspace/src/github.com/jmoiron/sqlx"
)

type StateMachine struct {
	State        int
	StateEntered bool
	Handlers     []State
	keys         []string
	pkgName      string
	logger       *log.Entry
	db           *sqlx.DB
	resetFn      func(*Msg)
}

type State struct {
	// OnEntry preprocesses and asks the user for information. If you need
	// to do something when the state begins, like run a search or hit an
	// endpoint, do that within the OnEntry function, since it's only called
	// once.
	OnEntry func(*Msg) string

	// OnInput sets the category in the cache/DB. Note that if invalid, this
	// state's Complete function will return false, preventing the user from
	// continuing. User messages will continue to hit this OnInput func
	// until Complete returns true.
	//
	// A note on error handling: errors should be logged but are not
	// propogated up to the user. Due to the preferred style of thin
	// States, you should generally avoid logging errors directly in
	// the OnInput function and instead log them within any called functions
	// (e.g. setPreference).
	OnInput func(*Msg)

	// Complete will determine if the state machine continues. If true,
	// it'll move to the next state. If false, the user's next response will
	// hit this state's OnInput function again.
	Complete func(*Msg) (bool, string)

	// Memory will search through preferences about the user. If a past
	// preference is found, it'll skip to the OnInput response, with that
	// preference as the input.
	Memory string
}

func NewStateMachine(pkgName string) (*StateMachine, error) {
	sm := StateMachine{State: 0, pkgName: pkgName}
	// TODO load state from DB
	sm.resetFn = func(*Msg) {}
	sm.logger = log.WithFields(log.Fields{
		"pkg": pkgName,
	})
	return &sm, nil
}

func (sm *StateMachine) SetStates(sss ...[]State) {
	for _, ss := range sss {
		for _, s := range ss {
			sm.Handlers = append(sm.Handlers, s)
		}
	}
}

func (sm *StateMachine) SetLogger(l *log.Entry) {
	sm.logger = l
}

func (sm *StateMachine) SetDBConn(s *sqlx.DB) {
	sm.db = s
}

func (sm *StateMachine) GetDBConn() *sqlx.DB {
	return sm.db
}

func (sm *StateMachine) SetPkgName(n string) {
	sm.pkgName = n
}

func (sm *StateMachine) Next(in *Msg) string {
	if sm.State+1 >= len(sm.Handlers) {
		sm.Reset(in)
		return sm.Handlers[sm.State].OnEntry(in)
	}
	if !sm.StateEntered {
		sm.StateEntered = true
		return sm.Handlers[sm.State].OnEntry(in)
	}
	// check completion of current state
	done, str := sm.Handlers[sm.State].Complete(in)
	if done {
		sm.State++
		s := sm.Handlers[sm.State].OnEntry(in)
		if len(s) == 0 {
			sm.logger.WithField("state", sm.State).
				Warnln("OnEntry returned \"\"")
		}
		return s
	} else if len(str) > 0 {
		return str
	}
	// check memory to determine if Ava should skip this state
	mem := sm.Handlers[sm.State].Memory
	if len(mem) > 0 {
		if sm.HasMemory(in, mem) {
			return sm.Next(in)
		}
	}
	sm.Handlers[sm.State].OnInput(in)
	return ""
}

func (sm *StateMachine) OnInput(in *Msg) {
	sm.Handlers[sm.State].OnInput(in)
}

func (sm *StateMachine) SetOnReset(reset func(in *Msg)) {
	sm.resetFn = reset
}

func (sm *StateMachine) SetMemory(in *Msg, k string, v interface{}) {
	b, err := json.Marshal(v)
	if err != nil {
		sm.logger.Errorln(err, "marhsalling interface to json", v)
		return
	}
	q := `INSERT INTO states (key, value, pkgname, userid)
	      VALUES ($1, $2, $3, $4)
	      ON CONFLICT (userid, pkgname, key) DO UPDATE SET value=$2`
	_, err := sm.db.Exec(q, k, b, sm.pkgName, in.User.ID)
	if err != nil {
		sm.logger.Errorln(err, "setting memory at", k, "to", v)
		return
	}
	// TODO set preference here as well
}

func (sm *StateMachine) GetMemory(in *Msg, k string) Memory {
	q := `SELECT value FROM states WHERE userid=$1 AND pkgname=$2 AND key=$3`
	var buf []byte
	err := sm.db.Get(&buf, q, in.User.ID, sm.pkgName, k)
	if err == sql.ErrNoRows {
		return Memory{Key: k, Val: json.RawMessage{}, logger: sm.logger}
	}
	if err != nil {
		sm.logger.Errorln(err, "getMemory for key", k)
		return Memory{Key: k, Val: json.RawMessage{}, logger: sm.logger}
	}
	return Memory{Key: k, Val: buf, logger: sm.logger}
}

func (sm *StateMachine) HasMemory(in *Msg, k string) bool {
	return len(sm.GetMemory(in, k).Val) > 0
}

func (sm *StateMachine) Reset(in *Msg) {
	sm.State = 0
	sm.StateEntered = false
	sm.resetFn(in)
}