<template>
  <div class="row">
    <AppList class="col-xs-0 col-sm"/>
    <div class="col-xs-12 col-sm-7 col-md-8">
      <router-view/>
    </div>
  </div>
</template>

<script lang="ts">
import { Options, Vue } from 'vue-class-component'
import AppList from '@/components/AppList.vue'

@Options({
  components: {
    AppList,
  },
})
export default class Apps extends Vue {

}
</script>
