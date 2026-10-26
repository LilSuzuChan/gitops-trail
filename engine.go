package main

import (
	"fmt"
	"sort"
)

func resourceKinds() []string {
	return []string{"applications.argoproj.io", "deployments", "replicasets", "pods", "events"}
}
func diagnose(r *Report, o, b []Obj, n, name, src, dst string, port int) {
	a := find(o, "Application", n, name)
	if a == nil {
		missing(r, "Application", n, name)
		return
	}
	for _, p := range []struct {
		id   string
		path []string
	}{{"revision", []string{"status", "sync", "revision"}}, {"sync", []string{"status", "sync", "status"}}, {"health", []string{"status", "health", "status"}}} {
		v := s(a, p.path...)
		cls := "confirmed"
		sev := "info"
		if v == "" {
			v = "unavailable"
			cls = "unknown"
			sev = "warning"
		}
		r.add(p.id, sev, cls, v, a, fmt.Sprint(p.path))
	}
	for _, c := range arr(a, "status", "conditions") {
		r.add("application-condition", "warning", "confirmed", s(c, "type"), a, "status.conditions")
	}
	if s(a, "status", "operationState", "phase") == "Failed" || s(a, "status", "operationState", "phase") == "Error" {
		r.add("sync-failed", "error", "confirmed", "Argo CD reports sync operation failure", a, "status.operationState.phase")
	}
	linked := arr(a, "status", "resources")
	if len(linked) == 0 {
		r.add("tracking-unavailable", "warning", "unknown", "Application has no tracked resource list", a, "status.resources")
	}
	for _, res := range linked {
		rn := s(res, "namespace")
		if rn == "" {
			rn = s(a, "spec", "destination", "namespace")
		}
		kind := s(res, "kind")
		obj := find(o, kind, rn, s(res, "name"))
		r.add("tracked-resource", "info", "confirmed", kind+"/"+rn+"/"+s(res, "name"), a, "status.resources")
		if obj == nil {
			missing(r, kind, rn, s(res, "name"))
			continue
		}
		if kind == "Deployment" {
			diagnoseDeployment(r, o, obj)
		}
	}
	times := []string{}
	for _, field := range []string{"startedAt", "finishedAt"} {
		if t := s(a, "status", "operationState", field); t != "" {
			times = append(times, t+" operation."+field)
		}
	}
	for _, ev := range o {
		if s(ev, "kind") != "Event" {
			continue
		}
		for _, res := range linked {
			target := find(o, s(res, "kind"), s(res, "namespace"), s(res, "name"))
			if target != nil && s(target, "metadata", "uid") != "" && s(ev, "involvedObject", "uid") == s(target, "metadata", "uid") {
				t := s(ev, "eventTime")
				if t == "" {
					t = s(ev, "lastTimestamp")
				}
				if t != "" {
					times = append(times, t+" "+s(ev, "reason"))
				}
			}
		}
	}
	sort.Strings(times)
	for _, t := range times {
		r.add("timeline", "info", "confirmed", t, a, "status.operationState / linked Events")
	}
	r.Limitations = append(r.Limitations, "Timeline may be incomplete; revision/image causality is not inferred. Live collection covers only --namespace; cross-namespace resources require a snapshot.")
}
