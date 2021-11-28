<template>
  <AppSettings :app="$store.state.getApp($route.params.app.toString())"/>
</template>

<script lang="ts">
import { Options, Vue } from 'vue-class-component';
import AppSettings from '@/components/AppSettings.vue';

@Options({
  components: {
    AppSettings,
  },
})
export default class App extends Vue {
}
</script>
