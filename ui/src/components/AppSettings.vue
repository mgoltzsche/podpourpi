<template>
  <div class="app-settings q-pa-md">
    <q-card>
      <q-card-section>
        name: {{app.metadata.name}}
        <q-btn-dropdown
          split
          class="glossy"
          color="teal"
          :label="toggleAppButtonLabel()"
          @click="toggleAppState(undefined)"
        >
          <q-list>
            <q-item clickable v-close-popup @click="toggleAppState(profile.name)" v-for="profile in profiles" :key="profile.name">
              <q-item-section avatar>
                <q-avatar icon="assignment" color="primary" text-color="white" />
              </q-item-section>
              <q-item-section>
                <q-item-label line="1">{{ profile.name }}</q-item-label>
                <q-item-label caption>nice profile</q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-icon name="info" color="amber" />
              </q-item-section>
            </q-item>
          </q-list>
        </q-btn-dropdown>
        <q-list>
          <q-item v-for="c in app.status.containers || []" :key="c.name">
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
import { Options, Vue } from 'vue-class-component'
import { stateColors } from './AppList.vue'
import {
  com_github_mgoltzsche_podpourpi_pkg_apis_app_v1alpha1_App as App,
  com_github_mgoltzsche_podpourpi_pkg_apis_app_v1alpha1_Container as Container,
  com_github_mgoltzsche_podpourpi_pkg_apis_app_v1alpha1_ContainerStatus as AppState,
} from '@/client'

class Profile {
  name: string
  constructor(name: string) {
    this.name = name
  }
}

@Options({
  props: {
    app: null,
  }
})
export default class AppSettings extends Vue {
  app!: App
  profiles: Profile[] = [{name: 'p1'}, {name: 'p2'}]
  toggleAppState(profile: string|undefined): void {
    // TODO: implement App update
    /*let toggleFn = (app: string) => {
      return AppsService.startApp(app, profile)
    }
    if (!this.isAppStopped()) {
      toggleFn = AppsService.stopApp
    }
    toggleFn(this.app.metadata.name).catch(e => {
      let msg = e instanceof ApiError ? `${e.toString()}: ${(e as ApiError).body?.message}` : e.toString()
      this.$q.notify({
        type: 'negative',
        message: msg,
      })
    })*/
  }
  isAppStopped(): boolean {
    return this.app.status.state == AppState.state.UNKNOWN || this.app.status.state == AppState.state.EXITED
  }
  toggleAppButtonLabel(): string {
    return (this.isAppStopped() ? 'start' : 'stop') + ` (${this.app.status.activeProfile})`
  }
  color(c: Container): string {
    return stateColors[c.status.state]
  }
  truncate(s: string, maxChars: number) {
    return s.length > maxChars ? s.substring(0, maxChars) : s
  }
}
</script>
