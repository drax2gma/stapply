package actions

import "github.com/drax2gma/stapply/internal/protocol"

// Action is the interface for all action executors.
type Action interface {
	// Execute runs the action and returns the response.
	Execute(requestID string, args map[string]string, dryRun bool) *protocol.RunResponse
}

// Registry holds registered action executors.
type Registry struct {
	actions map[string]Action
}

// NewRegistry creates a new action registry with default actions.
func NewRegistry() *Registry {
	r := &Registry{
		actions: make(map[string]Action),
	}
	// Register built-in actions
	r.Register("cmd", &CmdAction{})
	r.Register("write_file", &WriteFileAction{})
	r.Register("template_file", &TemplateFileAction{})
	r.Register("systemd", &SystemdAction{})
	r.Register("deploy_artifact", &DeployArtifactAction{})
	return r
}

// Register adds an action to the registry.
func (r *Registry) Register(name string, action Action) {
	r.actions[name] = action
}

// Get retrieves an action by name.
func (r *Registry) Get(name string) (Action, bool) {
	a, ok := r.actions[name]
	return a, ok
}

// Execute runs an action by name.
func (r *Registry) Execute(requestID, actionName string, args map[string]string, dryRun bool) *protocol.RunResponse {
	action, ok := r.Get(actionName)
	if !ok {
		return protocol.NewErrorResponse(requestID,
			&ActionError{Action: actionName, Err: ErrUnknownAction}, 0)
	}
	return action.Execute(requestID, args, dryRun)
}
