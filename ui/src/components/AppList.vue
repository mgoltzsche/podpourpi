<template>
  <div class="apps">
    <q-list>
      <q-item v-for="app in $store.state.apps" :key="app.metadata.name" clickable v-ripple :to="`/apps/${app.metadata.name}`">
        <q-item-section avatar>
          <q-avatar :color="color(app)" text-color="white">
          </q-avatar>
        </q-item-section>

        <q-item-section>
          <q-item-label lines="1">{{ app.metadata.name }}</q-item-label>
          <q-item-label caption lines="1">{{ app.status.containers?.length }} containers</q-item-label>
        </q-item-section>
          {{ app.status.state }}
        <q-item-section side>
        </q-item-section>
      </q-item>
    </q-list>
  </div>
</template>

<script lang="ts">
import { App } from '@/client';
import { Vue } from 'vue-class-component'
import { AppState } from '@/client';

let stateColors: {[key in AppState]: string} = {
  running: 'positive',
  error: 'negative',
  starting: 'info',
  unknown: 'dark',
  exited: 'gray'
}

export default class AppList extends Vue {
  color(a: App): string {
    return stateColors[a.status.state]
  }
}
</script>
