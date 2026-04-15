package main

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	defaultTailWaitSeconds = 3
	defaultTailMaxLines    = 200
	defaultDoneMaxLines    = 5000
	hardTailMaxLines       = 10000
	tailPollInterval       = 300 * time.Millisecond
)

type tailLine struct {
	Pod       string `json:"pod,omitempty"`
	Container string `json:"container"`
	Line      string `json:"line"`
	Index     int    `json:"index"`
}

type containerCursor struct {
	TS   string `json:"ts"`
	Skip int    `json:"skip"`
}

type podTailCursor struct {
	Containers map[string]containerCursor `json:"containers"`
}

type podLogTarget struct {
	Pod  *corev1.Pod
	Done bool
}

func toInt(v interface{}, fallback int) int {
	switch t := v.(type) {
	case int:
		return t
	case int8:
		return int(t)
	case int16:
		return int(t)
	case int32:
		return int(t)
	case int64:
		return int(t)
	case uint:
		return int(t)
	case uint8:
		return int(t)
	case uint16:
		return int(t)
	case uint32:
		return int(t)
	case uint64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	case string:
		i, err := strconv.Atoi(strings.TrimSpace(t))
		if err == nil {
			return i
		}
	}

	return fallback
}

func clampLimit(limit int) int {
	if limit <= 0 {
		limit = defaultTailMaxLines
	}
	if limit > hardTailMaxLines {
		limit = hardTailMaxLines
	}

	return limit
}

func parseIncludeContainers(payload map[string]interface{}) map[string]bool {
	ret := map[string]bool{}
	raw, ok := payload["include_containers"]
	if !ok || raw == nil {
		return ret
	}

	switch t := raw.(type) {
	case []interface{}:
		for _, v := range t {
			name := strings.TrimSpace(fmt.Sprintf("%v", v))
			if name != "" {
				ret[name] = true
			}
		}
	case []string:
		for _, v := range t {
			name := strings.TrimSpace(v)
			if name != "" {
				ret[name] = true
			}
		}
	}

	return ret
}

func parsePodsMapCursors(podsMap map[string]interface{}) map[string]podTailCursor {
	ret := map[string]podTailCursor{}
	for podName, podItemRaw := range podsMap {
		podCursor := podTailCursor{Containers: map[string]containerCursor{}}
		podMap, isPodMap := podItemRaw.(map[string]interface{})
		if !isPodMap {
			continue
		}
		containersRaw, foundContainers := podMap["containers"]
		if !foundContainers {
			continue
		}
		containersMap, isContainersMap := containersRaw.(map[string]interface{})
		if !isContainersMap {
			continue
		}
		for containerName, containerStateRaw := range containersMap {
			podCursor.Containers[containerName] = parseContainerCursorState(containerStateRaw)
		}
		ret[podName] = podCursor
	}

	return ret
}

func parseKubernetesCursor(payload map[string]interface{}) map[string]podTailCursor {
	ret := map[string]podTailCursor{}
	raw, ok := payload["cursor"]
	if !ok || raw == nil {
		return ret
	}

	cursorMap, ok := raw.(map[string]interface{})
	if !ok {
		return ret
	}

	if podsRaw, found := cursorMap["pods"]; found {
		if podsMap, isMap := podsRaw.(map[string]interface{}); isMap {
			return parsePodsMapCursors(podsMap)
		}
	}

	for name, item := range cursorMap {
		ret[name] = podTailCursor{Containers: map[string]containerCursor{name: parseContainerCursorState(item)}}
	}

	return ret
}

func parseContainerCursorState(raw interface{}) containerCursor {
	state := containerCursor{}
	switch t := raw.(type) {
	case map[string]interface{}:
		if ts, ok := t["ts"].(string); ok {
			state.TS = strings.TrimSpace(ts)
		}
		state.Skip = toInt(t["skip"], 0)
	default:
		state.Skip = toInt(t, 0)
	}

	if state.Skip < 0 {
		state.Skip = 0
	}

	return state
}

func extractLineTimestamp(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	space := strings.IndexByte(line, ' ')
	if space <= 0 {
		return ""
	}

	ts := line[:space]
	if _, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return ts
	}
	if _, err := time.Parse(time.RFC3339, ts); err == nil {
		return ts
	}

	return ""
}

func filterByCursor(lines []string, state containerCursor) []string {
	if state.TS == "" || state.Skip <= 0 {
		return lines
	}

	ret := []string{}
	boundarySeen := 0
	for _, line := range lines {
		ts := extractLineTimestamp(line)
		if ts == state.TS && boundarySeen < state.Skip {
			boundarySeen++

			continue
		}
		ret = append(ret, line)
	}

	return ret
}

func updateCursorWithLine(state containerCursor, line string) containerCursor {
	ts := extractLineTimestamp(line)
	if ts == "" {
		state.Skip++

		return state
	}

	if state.TS == ts {
		state.Skip++

		return state
	}

	state.TS = ts
	state.Skip = 1

	return state
}

func listPodLogTargets(m *dipper.Message, namespace, jobID string) ([]podLogTarget, string, string) {
	k8client := prepareKubeConfig(m)
	ctx, cancel := driver.GetContext(m)
	defer cancel()

	client := k8client.CoreV1().Pods(namespace)

	targets := []podLogTarget{}
	search := metav1.ListOptions{LabelSelector: "job-name==" + jobID}
	podList, err := client.List(ctx, search)
	if err != nil {
		log.Warningf("[%s] unable to list pods for job %s: %+v", driver.Service, jobID, err)

		return targets, StatusFailure, fmt.Sprintf("unable to list pods for job %s", jobID)
	}

	for i := range podList.Items {
		pod := podList.Items[i]
		targets = append(targets, podLogTarget{Pod: &pod, Done: podDone(&pod)})
	}

	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Pod.Name < targets[j].Pod.Name
	})

	status, reason := podStatusFromTargets(targets)

	return targets, status, reason
}

func podDone(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}

	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return true
	}

	statuses := append([]corev1.ContainerStatus{}, pod.Status.InitContainerStatuses...)
	statuses = append(statuses, pod.Status.ContainerStatuses...)
	if len(statuses) == 0 {
		return false
	}

	for _, st := range statuses {
		if st.State.Terminated == nil {
			return false
		}
	}

	return true
}

func podStatusFromTargets(targets []podLogTarget) (string, string) {
	if len(targets) == 0 {
		return StatusPending, "pod not found"
	}

	failureReason := ""
	for _, target := range targets {
		if !target.Done {
			return StatusPending, "pod is still running"
		}

		statuses := append([]corev1.ContainerStatus{}, target.Pod.Status.InitContainerStatuses...)
		statuses = append(statuses, target.Pod.Status.ContainerStatuses...)
		for _, st := range statuses {
			if st.State.Terminated != nil && st.State.Terminated.ExitCode != 0 {
				if failureReason == "" {
					failureReason = fmt.Sprintf("container %s.%s failed with exit code %d", target.Pod.Name, st.Name, st.State.Terminated.ExitCode)
				}
			}
		}
	}

	if failureReason != "" {
		return StatusFailure, failureReason
	}

	return StatusSuccess, ""
}

func fetchKubernetesContainerLines(
	m *dipper.Message, namespace, podName, containerName string, state containerCursor, tailLimit int,
) []string {
	k8client := prepareKubeConfig(m)
	ctx, cancel := driver.GetContext(m)
	defer cancel()

	opts := &corev1.PodLogOptions{Container: containerName, Timestamps: true}
	if state.TS != "" {
		if ts, err := time.Parse(time.RFC3339Nano, state.TS); err == nil {
			metaTime := metav1.NewTime(ts)
			opts.SinceTime = &metaTime
		} else if ts, err := time.Parse(time.RFC3339, state.TS); err == nil {
			metaTime := metav1.NewTime(ts)
			opts.SinceTime = &metaTime
		}
	} else if tailLimit > 0 {
		limit := int64(tailLimit)
		opts.TailLines = &limit
	}

	stream, err := k8client.CoreV1().Pods(namespace).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		log.Warningf("[%s] unable to stream logs for %s.%s: %+v", driver.Service, podName, containerName, err)

		return []string{}
	}
	defer stream.Close()

	return readLogLines(stream)
}

func readLogLines(stream io.Reader) []string {
	ret := []string{}
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		ret = append(ret, scanner.Text())
	}

	return ret
}

func buildTailChunkForPods(
	m *dipper.Message, namespace, jobID string,
	cursor map[string]podTailCursor, include map[string]bool,
	limit int, tailForEmptyCursor int,
) ([]tailLine, map[string]podTailCursor, bool, bool, string, string) {
	targets, status, reason := listPodLogTargets(m, namespace, jobID)
	done := status == StatusSuccess || status == StatusFailure

	lines := []tailLine{}
	nextCursor := map[string]podTailCursor{}
	hasMore := false

	for _, target := range targets {
		podName := target.Pod.Name
		podCursor := cursor[podName]
		if podCursor.Containers == nil {
			podCursor.Containers = map[string]containerCursor{}
		}

		containers := append([]corev1.Container{}, target.Pod.Spec.InitContainers...)
		containers = append(containers, target.Pod.Spec.Containers...)
		sort.Slice(containers, func(i, j int) bool {
			return containers[i].Name < containers[j].Name
		})

		for _, container := range containers {
			if len(include) > 0 && !include[container.Name] {
				continue
			}

			state := podCursor.Containers[container.Name]
			rawLines := fetchKubernetesContainerLines(m, namespace, podName, container.Name, state, tailForEmptyCursor)
			candidateLines := filterByCursor(rawLines, state)

			remaining := limit - len(lines)
			emitCount := len(candidateLines)
			if remaining < emitCount {
				emitCount = remaining
			}

			for i := 0; i < emitCount; i++ {
				line := candidateLines[i]
				lines = append(lines, tailLine{
					Pod:       podName,
					Container: container.Name,
					Line:      line,
					Index:     state.Skip,
				})
				state = updateCursorWithLine(state, line)
			}

			podCursor.Containers[container.Name] = state
			if emitCount < len(candidateLines) {
				hasMore = true
			}

			if len(lines) >= limit {
				hasMore = true

				break
			}
		}

		nextCursor[podName] = podCursor
		if len(lines) >= limit {
			break
		}
	}

	for podName, state := range cursor {
		if _, ok := nextCursor[podName]; !ok {
			nextCursor[podName] = state
		}
	}

	if len(lines) > 0 && status == StatusPending {
		status = StatusSuccess
		reason = ""
	}

	return lines, nextCursor, hasMore, done, status, reason
}

func getPodLogTail(msg *dipper.Message) {
	msg = dipper.DeserializePayload(msg)
	payload, ok := msg.Payload.(map[string]interface{})
	if !ok {
		payload = map[string]interface{}{}
	}

	namespace, ok := dipper.GetMapDataStr(payload, "namespace")
	if !ok {
		namespace = DefaultNamespace
	}
	// Accept pod_id from operator API (which actually contains the job ID for Kubernetes)
	jobID := dipper.MustGetMapDataStr(payload, "pod_id")

	waitSeconds := toInt(payload["wait_seconds"], defaultTailWaitSeconds)
	if waitSeconds < 0 {
		waitSeconds = 0
	}
	maxLines := clampLimit(toInt(payload["max_lines"], defaultTailMaxLines))
	doneMaxLines := clampLimit(toInt(payload["done_max_lines"], defaultDoneMaxLines))
	cursor := parseKubernetesCursor(payload)
	include := parseIncludeContainers(payload)

	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	for {
		lines, nextCursor, hasMore, done, status, reason := buildTailChunkForPods(msg, namespace, jobID, cursor, include, maxLines, maxLines)
		limitUsed := maxLines
		if done {
			limitUsed = doneMaxLines
			if limitUsed != maxLines {
				lines, nextCursor, hasMore, done, status, reason = buildTailChunkForPods(msg, namespace, jobID, cursor, include, limitUsed, doneMaxLines)
			}
		}

		if len(lines) > 0 || done || waitSeconds == 0 || time.Now().After(deadline) {
			msg.Reply <- dipper.Message{
				Payload: map[string]interface{}{
					"lines":       lines,
					"next_cursor": map[string]interface{}{"pods": nextCursor},
					"has_more":    hasMore,
					"done":        done,
					"truncated":   hasMore && len(lines) >= limitUsed,
				},
				Labels: map[string]string{
					"status": status,
					"reason": reason,
				},
			}

			return
		}

		time.Sleep(tailPollInterval)
	}
}
