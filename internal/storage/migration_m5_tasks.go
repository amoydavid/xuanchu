package storage

import "strings"

var m5TaskColumns = []string{
	"uuid", "workspace_id", "description", "status", "entry", "modified",
	"end_ts", "due", "project", "priority",
	"start", "wait", "scheduled", "until",
	"recur", "parent", "mask", "i_mask",
	"project_seq",
}

var m4TaskIndexes = []string{
	"CREATE INDEX IF NOT EXISTS idx_tasks_workspace_id ON tasks(workspace_id)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_wait ON tasks(wait)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_scheduled ON tasks(scheduled)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_until ON tasks(until)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_recur ON tasks(recur)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent)",
	"CREATE UNIQUE INDEX IF NOT EXISTS idx_task_parent_due_open ON tasks(parent, due) WHERE status IN ('pending', 'waiting') AND parent IS NOT NULL AND due IS NOT NULL",
}

var m4TaskIndexNames = []string{
	"idx_tasks_workspace_id",
	"idx_tasks_status",
	"idx_tasks_project",
	"idx_tasks_wait",
	"idx_tasks_scheduled",
	"idx_tasks_until",
	"idx_tasks_recur",
	"idx_tasks_parent",
	"idx_task_parent_due_open",
}

var m5TaskIndexes = []string{
	"CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_wait ON tasks(wait)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_scheduled ON tasks(scheduled)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_until ON tasks(until)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_recur ON tasks(recur)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent)",
	"CREATE INDEX IF NOT EXISTS idx_tasks_ws_project_id ON tasks(workspace_id, project_id)",
	"CREATE UNIQUE INDEX IF NOT EXISTS idx_task_parent_due_open ON tasks(parent, due) WHERE status IN ('pending', 'waiting') AND parent IS NOT NULL AND due IS NOT NULL",
}

var m4TasksDDL = `CREATE TABLE tasks (
	uuid TEXT PRIMARY KEY,
	workspace_id TEXT NOT NULL,
	description TEXT NOT NULL,
	status TEXT NOT NULL,
	entry INTEGER NOT NULL,
	modified INTEGER NOT NULL,
	end_ts INTEGER,
	due INTEGER,
	project TEXT,
	priority TEXT,
	start INTEGER,
	wait INTEGER,
	scheduled INTEGER,
	until INTEGER,
	recur TEXT,
	parent TEXT,
	mask TEXT,
	i_mask INTEGER
)`

var m5TasksDDL = strings.TrimSuffix(strings.TrimSuffix(m4TasksDDL, "\n)"), ")") + `,
	project_seq INTEGER,
	project_id TEXT,
	FOREIGN KEY (project_id, workspace_id) REFERENCES projects(id, workspace_id)
)`
