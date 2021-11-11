<template>
  <AppList/>
</template>

<script lang="ts">
import { Options, Vue } from 'vue-class-component';
import AppList from '@/components/AppList.vue';

@Options({
  components: {
    AppList,
  },
})
export default class Apps extends Vue {

}
</script>
