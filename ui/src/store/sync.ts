import { MutationTypes } from '../store/mutations'
import { EventList } from '../client/models/EventList'
import { store } from './store'

export function watchServerChanges(): void {
  streamFromURL<EventList>('/api/v1/events').then(stream => {
    return consumeStream(stream, evt => {
      console.log('sync: received event:', evt)
      store.commit(MutationTypes.SYNC_DATA, evt.items)
    })
  }).catch(e => {
    console.log('ERROR: sync failed:', new Date(), e)
    console.log('sync: restarting...')
    setTimeout(watchServerChanges, 1000) // retry after 1s
  })
}

async function consumeStream<T>(stream: ReadableStream<T>, consume: (value:T)=>void) {
  const reader = stream.getReader()
  let value: T | undefined
  let done = false
  while (!done) {
    ({ value, done } = await reader.read())
    if (done) {
      console.log('ERROR: sync: server terminated stream')
      return
    }
    if (value) consume(value)
  }
}

function streamFromURL<T>(url: string): Promise<ReadableStream<T>> {
  return fetch(url).then((response) => {
    if (response.status != 200) {
      return Promise.reject(`request ${url}: server responded with status code ${response.status}`)
    }
    if (!response.body) {
      return Promise.reject(`request ${url}: missing response body`)
    }
    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let text = ''
    const stream = new ReadableStream<T>({
      start(controller: ReadableStreamController<T>) {
        function push(): Promise<void> {
          return reader.read().then(({ done, value }) => {
            if (done) {
              controller.close()
              return
            }
            text += decoder.decode(value || new Uint8Array)
            for (let pos = text.indexOf('\n'); pos > -1; pos = text.indexOf('\n')) {
              const line = text.substring(0, pos).trim()
              if (line) {
                controller.enqueue(JSON.parse(line))
              }
              text = text.substring(pos+1)
            }
          }).then(push).catch(e => {
            controller.error(e)
          })
        }
        push()
      },
      cancel() {
        // TODO: check if any of this fails and reorder and/or catch it        
        reader.cancel()
        response.body?.cancel()
      },
    })
    return stream
  })
}

/*
function consumeMessage(rawMsg: string) {
  const events: EventList = JSON.parse(rawMsg)
  console.log('sync: received message:', events.items)
  store.commit(MutationTypes.SYNC_DATA, events.items)
}

function handleWatchError(error: Error) {
  console.log('ERROR: sync failed:', new Date(), error)
  console.log('sync: restarting...')
  setTimeout(watchServerChanges, 1000) // retry after 1s
}

function streamHttpLines(url: string, lineConsumer: (msg: string)=>void, errHandler: (err: Error)=>void) {
  return fetch(url)
    .then(readChunkedResponse)
    .catch(errHandler)

  async function readChunkedResponse(response: Response) {
    if (response.body == null) {
      console.log(`ERROR: response body of ${url} request is null`)
      return
    }
    const reader = response.body.getReader()
    const decoder = new TextDecoder()
    let value: Uint8Array | undefined
    let done = false
    let text = ''
    while (!done) {
      ({ value, done } = await reader.read())
      if (done) {
        console.log('ERROR: sync: server terminated sync stream')
        return
      }
      text += decoder.decode(value || new Uint8Array)
      for (let pos = text.indexOf('\n'); pos > -1; pos = text.indexOf('\n')) {
        consumeLine(text.substring(0, pos))
        text = text.substring(pos+1)
      }
    }
  }

  function consumeLine(msg: string) {
    msg = msg.trim()
    if (msg) {
      try {
        lineConsumer(msg)
      } catch(e) {
        console.log('ERROR: sync: consumer error:', e)
      }
    }
  }
}*/
