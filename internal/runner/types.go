package runner

import (
	"fmt"

	"github.com/mgoltzsche/podpourpi/internal/store"
)

type AppState int

const (
	AppStateUnknown  AppState = iota
	AppStateDisabled AppState = iota
	AppStateExited   AppState = iota
	AppStateReady    AppState = iota
	AppStateStarting AppState = iota
	AppStateStopping AppState = iota
	AppStateError    AppState = iota
)

var (
	states = []string{
		"unknown",
		"disabled",
		"exited",
		"ready",
		"starting",
		"stopping",
		"error",
	}
)

func (a AppState) String() string {
	return states[int(a)]
}

// +k8s:deepcopy-gen=true
type App struct {
	Name    string
	Type    string
	Host    string
	Enabled bool
	Status  AppStatus
}

func (a *App) DeepCopyIntoResource(dest store.Resource) error {
	out, ok := dest.(*App)
	if !ok {
		return fmt.Errorf("deep copy: unexpected target type %T provided, expected %T", dest, a)
	}
	a.DeepCopyInto(out)
	return nil
}

// +k8s:deepcopy-gen=true
type AppList struct {
	Items []App
}

func (a *AppList) SetItems(items []store.Resource) error {
	a.Items = make([]App, len(items))
	for i, item := range items {
		if err := item.DeepCopyIntoResource(&a.Items[i]); err != nil {
			return err
		}
	}
	return nil
}

// +k8s:deepcopy-gen=true
type AppStatus struct {
	ActiveProfile string
	State         AppState
	Containers    []AppContainer
}

// +k8s:deepcopy-gen=true
type AppContainer struct {
	ID    string
	Name  string
	State AppState
}

// +k8s:deepcopy-gen=true
type Profile struct {
	Name       string
	App        string
	Properties []Property
}

// +k8s:deepcopy-gen=true
type Property struct {
	Name  string
	Value string
}

func (p *Profile) DeepCopyIntoResource(dest store.Resource) error {
	out, ok := dest.(*Profile)
	if !ok {
		return fmt.Errorf("deep copy: unexpected target type %T provided, expected %T", dest, p)
	}
	p.DeepCopyInto(out)
	return nil
}

// +k8s:deepcopy-gen=true
type ProfileList struct {
	Items []Profile
}

func (l *ProfileList) SetItems(items []store.Resource) error {
	l.Items = make([]Profile, len(items))
	for i, item := range items {
		if err := item.DeepCopyIntoResource(&l.Items[i]); err != nil {
			return err
		}
	}
	return nil
}
