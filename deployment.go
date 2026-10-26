package main

import "fmt"

func owned(o, parent Obj) bool {
	uid := s(parent, "metadata", "uid")
	if uid == "" || ns(o) != ns(parent) {
		return false
	}
	for _, owner := range arr(o, "metadata", "ownerReferences") {
		if s(owner, "uid") == uid && s(owner, "kind") == s(parent, "kind") {
			return true
		}
	}
	return false
}
func diagnoseDeployment(r *Report, objects []Obj, d Obj) {
	if get(d, "status") == nil || get(d, "status", "observedGeneration") == nil {
		r.add("deployment-status-unavailable", "warning", "unknown", "Deployment status or observedGeneration is missing", d, "status")
		return
	}
	if num(d, "status", "observedGeneration") < num(d, "metadata", "generation") {
		r.add("generation-unobserved", "info", "confirmed", "Controller has not observed the current generation", d, "status.observedGeneration")
		return
	}
	failed := false
	for _, c := range arr(d, "status", "conditions") {
		if s(c, "type") == "Progressing" && s(c, "status") == "False" && s(c, "reason") == "ProgressDeadlineExceeded" {
			failed = true
			r.add("progress-deadline-exceeded", "error", "confirmed", "Deployment reported ProgressDeadlineExceeded", d, "status.conditions")
		}
	}
	desired := 1
	if get(d, "spec", "replicas") != nil {
		desired = num(d, "spec", "replicas")
	}
	ready := num(d, "status", "availableReplicas") >= desired && num(d, "status", "updatedReplicas") == desired && num(d, "status", "replicas") == desired
	if !failed {
		if ready {
			r.add("rollout-complete", "info", "confirmed", "Observed replica counts satisfy the requested rollout", d, "status")
		} else {
			r.add("rollout-in-progress", "info", "confirmed", "Rollout is incomplete; this alone does not establish a failure", d, "status")
		}
	}
	rsFound := false
	for _, rs := range objects {
		if s(rs, "kind") != "ReplicaSet" || !owned(rs, d) {
			continue
		}
		rsFound = true
		r.add("replicaset-linked", "info", "confirmed", fmt.Sprintf("ReplicaSet %s revision %s", s(rs, "metadata", "name"), s(rs, "metadata", "annotations", "deployment.kubernetes.io/revision")), rs, "metadata.ownerReferences")
		for _, p := range objects {
			if s(p, "kind") != "Pod" || !owned(p, rs) {
				continue
			}
			for _, field := range []string{"containerStatuses", "initContainerStatuses"} {
				for _, c := range arr(p, "status", field) {
					reason := s(c, "state", "waiting", "reason")
					if reason != "" {
						severity := "warning"
						if reason == "ImagePullBackOff" || reason == "ErrImagePull" || reason == "CrashLoopBackOff" {
							severity = "error"
						}
						r.add("container-waiting", severity, "confirmed", s(c, "name")+": "+reason, p, "status."+field)
					}
					if num(c, "state", "terminated", "exitCode") != 0 {
						r.add("container-terminated", "error", "confirmed", s(c, "name")+": nonzero exit code", p, "status."+field)
					}
				}
			}
			for _, ev := range objects {
				if s(ev, "kind") == "Event" && s(p, "metadata", "uid") != "" && s(ev, "involvedObject", "uid") == s(p, "metadata", "uid") {
					r.add("pod-event", "info", "confirmed", s(ev, "reason")+" (message omitted to avoid exposing sensitive values)", ev, "reason")
				}
			}
		}
	}
	if !rsFound {
		r.add("replicaset-unavailable", "warning", "unknown", "No owned ReplicaSet available; Pod diagnosis is incomplete", d, "metadata.uid")
	}
}
