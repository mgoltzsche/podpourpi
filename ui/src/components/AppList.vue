<template>
  <div class="apps">
    <h1>Apps</h1>
    <ul id="appslist">
      <li v-for="app in $store.state.apps" :key="app.metadata.name">
        {{app.metadata.name}}({{app.status.containers?.length}})
      </li>
    </ul>
  </div>
</template>

<script lang="ts">
import { Vue } from 'vue-class-component'

export default class AppList extends Vue {}
</script>

<!-- Add "scoped" attribute to limit CSS to this component only -->
<style scoped lang="scss">
h3 {
  margin: 40px 0 0;
}
ul {
  list-style-type: none;
  padding: 0;
}
li {
  display: inline-block;
  margin: 0 10px;
}
a {
  color: #42b983;
}
</style>
