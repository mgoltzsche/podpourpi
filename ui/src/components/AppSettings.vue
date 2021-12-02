<template>
  <div class="app-settings q-pa-md">
    <q-card>
      <q-card-section>
        name: {{app.metadata.name}}
        <div class="q-gutter-sm">
          <q-checkbox v-model="app.spec.enabled" label="Enabled" @click="updateApp()"/>
        </div>
        <q-list>
          <q-item v-for="c in app.status.containers" :key="c.name">
            <q-item-section avatar>
              <q-avatar :color="color(c)" text-color="white">
              </q-avatar>
            </q-item-section>

            <q-item-section>
              <q-item-label lines="1">{{ c.name }} ({{ truncate(c.id, 7) }})</q-item-label>
            </q-item-section>
            <q-item-section side>{{ c.status.state }}</q-item-section>
          </q-item>
        </q-list>
      </q-card-section>
    </q-card>
  </div>
</template>

<script lang="ts">
import { ApiError, App, AppsService, Container } from '@/client'
import { Options, Vue } from 'vue-class-component'
import { stateColors } from './AppList.vue'

@Options({
  props: {
    app: null,
  }
})
export default class AppSettings extends Vue {
  app!: App

  updateApp(): void {
    console.log(`app ${this.app.metadata.name} enabled: ${this.app.spec.enabled}`)
    AppsService.updateApp(this.app.metadata.name, this.app).catch(e => {
      let msg = e instanceof ApiError ? `${e.toString()}: ${(e as ApiError).body?.message}` : e.toString()
      this.$q.notify({
        type: 'negative',
        message: msg,
      })
    })
  }
  color(c: Container): string {
    return stateColors[c.status.state]
  }
  truncate(s: string, maxChars: number) {
    return s.length > maxChars ? s.substring(0, maxChars) : s
  }
}
</script>
