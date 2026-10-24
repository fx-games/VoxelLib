package actors

// Task is one entry of a Tasks list: the reference's AI task (shouldExecute,
// continueExecuting, startExecuting, resetTask, updateTask) with its
// priority and mutex bits. Nil funcs do nothing; a nil Continue is Should.
type Task struct {
	Priority, Mutex int
	Should          func() bool
	Continue        func() bool
	Start           func()
	Reset           func()
	Update          func()
	Uninterruptible bool // isInterruptible false
}

// Tasks runs a list of tasks as the reference's EntityAITasks: every
// third Run the list is walked in order, a running task that may no
// longer run or continue is reset and may start again at once, and a
// task starts when no running task of the same or a higher priority (a
// lower number) shares a mutex bit with it (an uninterruptible running
// one of a lower priority also holds it off); on the other Runs only the
// running tasks' Continue is asked. The tasks started this Run start,
// then every running task updates, in the order they started.
type Tasks struct {
	List []*Task
	exec []*Task
	tick int
}

// Add appends a task (addTask).
func (t *Tasks) Add(task *Task) { t.List = append(t.List, task) }

// Running reports whether task is executing.
func (t *Tasks) Running(task *Task) bool {
	for _, e := range t.exec {
		if e == task {
			return true
		}
	}
	return false
}

func (t *Tasks) drop(task *Task) {
	for i, e := range t.exec {
		if e == task {
			t.exec = append(t.exec[:i], t.exec[i+1:]...)
			return
		}
	}
}

func call(f func()) {
	if f != nil {
		f()
	}
}

func (task *Task) cont() bool {
	if task.Continue != nil {
		return task.Continue()
	}
	return task.Should != nil && task.Should()
}

func (t *Tasks) canUse(task *Task) bool {
	for _, o := range t.List {
		if o == task || !t.Running(o) {
			continue
		}
		if task.Priority >= o.Priority {
			if task.Mutex&o.Mutex != 0 {
				return false
			}
		} else if o.Uninterruptible {
			return false
		}
	}
	return true
}

// Run is onUpdateTasks.
func (t *Tasks) Run() {
	var started []*Task
	if t.tick%3 == 0 {
		for _, task := range t.List {
			if t.Running(task) {
				if t.canUse(task) && task.cont() {
					continue
				}
				call(task.Reset)
				t.drop(task)
			}
			if t.canUse(task) && task.Should != nil && task.Should() {
				started = append(started, task)
				t.exec = append(t.exec, task)
			}
		}
	} else {
		for i := 0; i < len(t.exec); i++ {
			if task := t.exec[i]; !task.cont() {
				call(task.Reset)
				t.exec = append(t.exec[:i], t.exec[i+1:]...)
				i--
			}
		}
	}
	t.tick++
	for _, task := range started {
		call(task.Start)
	}
	for _, task := range t.exec {
		call(task.Update)
	}
}

// Stop resets every running task (the entity is gone or its tasks are
// replaced).
func (t *Tasks) Stop() {
	for _, task := range t.exec {
		call(task.Reset)
	}
	t.exec = nil
}
