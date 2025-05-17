import { ref, onMounted, onUnmounted } from 'vue'

interface EvaluationRequest {
  id: number
  event: 'evaluation_request'
  data: {
    positions: string[]
  }
}

interface EvaluationResponse {
  id: number
  data: {
    evaluations: Array<{
      position: string
      level: number
      depth: number
      confidence: number
      score: number
      best_moves: number[]
    }>
  }
}

export function useWebSocket() {
  const ws = ref<WebSocket | null>(null)
  const isConnected = ref(false)
  const requestId = ref(1)
  const pendingRequests = new Map<number, (evaluations: Map<string, number>) => void>()

  function connect() {
    ws.value = new WebSocket('wss://flippy.site/ws') // TODO don't hardcode this

    ws.value.onopen = () => {
      isConnected.value = true
    }

    ws.value.onclose = () => {
      isConnected.value = false
      // Try to reconnect after 1 second
      setTimeout(connect, 1000)
    }

    ws.value.onmessage = (event) => {
      const response = JSON.parse(event.data) as EvaluationResponse
      const callback = pendingRequests.get(response.id)
      if (callback) {
        const evaluations = new Map<string, number>()
        response.data.evaluations.forEach((evaluation) => {
          evaluations.set(evaluation.position, evaluation.score)
        })
        callback(evaluations)
        pendingRequests.delete(response.id)
      }
    }
  }

  function requestEvaluations(positions: string[]): Promise<Map<string, number>> {
    return new Promise((resolve) => {
      if (!ws.value || !isConnected.value) {
        resolve(new Map())
        return
      }

      const id = requestId.value++
      const request: EvaluationRequest = {
        id,
        event: 'evaluation_request',
        data: { positions },
      }

      pendingRequests.set(id, resolve)
      ws.value.send(JSON.stringify(request))
    })
  }

  onMounted(() => {
    connect()
  })

  onUnmounted(() => {
    if (ws.value) {
      ws.value.close()
    }
  })

  return {
    isConnected,
    requestEvaluations,
  }
}
