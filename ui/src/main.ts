import { createApp } from 'vue'
import App from './App.vue'
import router from './router'
import { store, key } from './store'
import { MutationTypes } from './store/mutations'
import { EventList } from './client/models/EventList'

const app = createApp(App)

app.use(store, key).use(router).mount('#app')

watchServerChanges()

function watchServerChanges() {
  fetch('/api/v1/events')
    .then(readChunkedResponse)
    .catch(handleWatchError)
}

function readChunkedResponse(response: Response) {
  if (response.body == null) {
      return Promise.reject('response body is null')
  }

  let text = ''
  const reader = response.body.getReader()
  const decoder = new TextDecoder()

  return readChunk()

  function readChunk() {
    return reader.read().then(handleChunk)
  }

  function handleChunk(result: ReadableStreamDefaultReadResult<Uint8Array>): Promise<void> {
    text += decoder.decode(result.value || new Uint8Array, {stream: !result.done})
    const lineBreakPos = text.indexOf("\n")
    if (lineBreakPos > 0) {
      handleMessage(text.substring(0, lineBreakPos))
      text = text.substring(lineBreakPos+1)
    }
    if (result.done) {
      console.log('server sync stopped')
      handleMessage(text)
      return Promise.resolve()
    } else {
      console.log('waiting for next message chunk')
      return readChunk()
    }
  }

  function handleMessage(rawMsg: string) {
    rawMsg = rawMsg.trim()
    if (rawMsg === '') {
      return
    }
    const events: EventList = JSON.parse(rawMsg)
    console.log('message:', events.items)
    store.commit(MutationTypes.SYNC_DATA, events.items)
  }
}

function handleWatchError(error: Error) {
  console.log('ERROR: server sync failed: ', error)
  setTimeout(watchServerChanges, 1000) // retry after 1s
}
