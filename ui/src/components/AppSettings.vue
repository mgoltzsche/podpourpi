<template>
  <div class="app-settings">
    <q-card>
      <q-card-section>
        <div v-if="app === null">
          App not specified
        </div>
        <div v-if="app !== null">
          TODO: app details for {{app.metadata.name}}
        </div>
        name: {{app?.metadata.name}}
      </q-card-section>
    </q-card>
  </div>
</template>

<script lang="ts">
import { App } from '@/client'
import { Options, Vue } from 'vue-class-component'

@Options({
  props: {
    app: null,
  }
})
export default class AppSettings extends Vue {
  app!: App|null
}
</script>
