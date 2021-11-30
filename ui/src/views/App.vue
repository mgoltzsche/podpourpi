<template>
  <AppSettings :app="app()" v-if="app() !== null"/>
  <div v-if="app() === null">
    App not found
  </div>
</template>

<script lang="ts">
import { Options, Vue } from 'vue-class-component';
import AppSettings from '@/components/AppSettings.vue';
import { App } from '@/client';

@Options({
  components: {
    AppSettings,
  },
})
export default class AppDetails extends Vue {
  app(): App | null {
    return this.$store.state.getApp(this.$route.params.app.toString())
  }
}
</script>
