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

// Singleton instance
let wsInstance: WebSocket | null = null
let isConnectedInstance = false
let requestIdInstance = 1
const pendingRequestsInstance = new Map<number, (evaluations: Map<string, number>) => void>()

// Ensure we only create one connection
let connectionPromise: Promise<void> | null = null

function connect() {
  if (connectionPromise) return connectionPromise

  connectionPromise = new Promise((resolve) => {
    if (wsInstance) {
      resolve()
      return
    }

    wsInstance = new WebSocket('wss://flippy.site/ws') // TODO don't hardcode this

    wsInstance.onopen = () => {
      isConnectedInstance = true
      resolve()
    }

    wsInstance.onclose = () => {
      isConnectedInstance = false
      wsInstance = null
      connectionPromise = null
      // Try to reconnect after 1 second
      setTimeout(connect, 1000)
    }

    wsInstance.onmessage = (event) => {
      const response = JSON.parse(event.data) as EvaluationResponse
      const callback = pendingRequestsInstance.get(response.id)
      if (callback) {
        const evaluations = new Map<string, number>()
        response.data.evaluations.forEach((evaluation) => {
          evaluations.set(evaluation.position, evaluation.score)
        })
        callback(evaluations)
        pendingRequestsInstance.delete(response.id)
      }
    }
  })

  return connectionPromise
}

// Initialize connection immediately
const initialConnection = connect()

export async function requestEvaluations(positions: string[]): Promise<Map<string, number>> {
  // Wait for the initial connection
  await initialConnection

  return new Promise((resolve) => {
    if (!wsInstance || !isConnectedInstance) {
      resolve(new Map())
      return
    }

    const id = requestIdInstance++
    const request: EvaluationRequest = {
      id,
      event: 'evaluation_request',
      data: { positions },
    }

    pendingRequestsInstance.set(id, resolve)
    wsInstance.send(JSON.stringify(request))
  })
}

export function isConnected(): boolean {
  return isConnectedInstance
}
