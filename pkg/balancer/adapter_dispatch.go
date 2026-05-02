package balancer

import (
	"FlakyOllama/pkg/balancer/adapters"
	"FlakyOllama/pkg/shared/models"
	"net/http"
)

func (b *Balancer) DispatchInference(w http.ResponseWriter, r *http.Request, agentPath string, body []byte, model, contextHash string, allowHedging bool, t adapters.Translator) {
	priority := b.getRequestPriority(r)
	surge := 1.0 + (float64(b.Queue.QueueDepth()) * 0.02)

	resp, _, agentAddr, err := b.DoHedgedRequest(r.Context(), model, agentPath, body, r.RemoteAddr, allowHedging, priority, contextHash)
	if err != nil {
		b.jsonError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	defer resp.Body.Close()

	b.finalizeProxyWithAdapter(w, resp, agentAddr, model, r, surge, t)
}

func (b *Balancer) ListModels() []adapters.AvailableModel {
	snap := b.State.GetSnapshot()
	uniqueModels := make(map[string]bool)
	for _, node := range snap.Agents {
		if node.State != models.StateBroken && !node.Draining {
			for _, m := range node.LocalModels {
				uniqueModels[m.Name] = true
			}
		}
	}

	b.configMu.RLock()
	virtualModels := b.Config.VirtualModels
	for m := range virtualModels {
		uniqueModels[m] = true
	}
	b.configMu.RUnlock()

	result := make([]adapters.AvailableModel, 0, len(uniqueModels))
	for name := range uniqueModels {
		err, isVirtual := virtualModels[name]
		// Skip virtual models that are not routable (e.g., pipelines with no valid targets), since we don't want to advertise them in the tag endpoint and then have requests fail later at routing.
		// They will still be "available" when generating, but will either fail or use a different available model at routing time, which is arguably better than advertising them and then having all requests for that model fail.
		// This way, v models with issues will gracefully be handled without being advertised as available, and operators can fix them without causing a flood of failed requests.

		if !err.IsRoutable() {
			continue
		}

		result = append(result, adapters.AvailableModel{Name: name, IsVirtual: isVirtual})
	}
	return result
}
