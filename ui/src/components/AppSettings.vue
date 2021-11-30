<template>
  <div class="app-settings q-pa-md">
    <q-card>
      <q-card-section>
        name: {{app.metadata.name}}
        <div class="q-gutter-sm">
          <q-checkbox v-model="app.spec.enabled" label="Enabled" @click="updateApp()"/>
        </div>
      </q-card-section>
    </q-card>
  </div>
</template>

<script lang="ts">
import { ApiError, App, AppsService } from '@/client'
import { Options, Vue } from 'vue-class-component'

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
}
</script>
