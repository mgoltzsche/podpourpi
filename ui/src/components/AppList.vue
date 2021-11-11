<template>
  <div class="apps">
    <h1>Apps</h1>
    <ul id="appslist">
      <li v-for="app in apps" :key="app.id">
        {{ app.id }}
      </li>
    </ul>
  </div>
</template>

<script lang="ts">
import { Options, Vue } from 'vue-class-component';

class App {
	id: string
	constructor(id: string) {
		this.id = id
	}
}

@Options({
  props: {
  }
})
export default class AppList extends Vue {
  apps: App[] = [{id: "fake-app1"}, {id: "fake-app2"}]
}
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
