package platform

/*
type WorkerIDGetter interface {
	// label is either IP or MAC. This is used to get the workerID which is just the hardware data ID
	GetWorkerID(ctx context.Context, label string) (workerID string, err error)
	//IdentifyWorkerByMac(ctx context.Context, mac string) (workerID string, err error)
	//IdentifyWorkerByLabel(ctx context.Context, label string) (workerID string, err error)
}

type WorkflowIDGetter interface {
	// GetWorkflows queries tink server to know whether there is a workflow task to run and what the details of that task are.
	// The only business/domain logic for GetAction is if err == nil, then run the actions
	// ideally tink server holds all the business logic here. tinklet shouldnt have to make any determinations about whether to run or wait.
	// if tink server provides a workflow.WorkflowActionList to run, it runs immediately.
	GetWorkflowIDs(ctx context.Context, workerID string) (actions []*workflow.WorkflowContext, err error)
}

type WorkflowActionsGetter interface {
	GetWorkflowActions(ctx context.Context, workflowID string, workerID string) ([]*workflow.WorkflowAction, error)
}

type ActionRunner interface {
	// RunAction will take workflow actions and execute them
	RunAction(ctx context.Context, action *workflow.WorkflowAction) error
}

type StatusReporter interface {
	ReportStatus(ctx context.Context, status *workflow.WorkflowActionStatus) error
}
*/
