<template>
  <div class="apps">
    <q-list>
      <q-item v-for="app in $store.state.apps" :key="app.metadata.name" clickable v-ripple :to="`/apps/${app.metadata.name}`">
        <q-item-section avatar>
          <q-avatar color="primary" text-color="white">
            {{ app.status.phase }}
          </q-avatar>
        </q-item-section>

        <q-item-section>
          <q-item-label lines="1">{{ app.metadata.name }}</q-item-label>
          <q-item-label caption lines="1">{{app.status.containers?.length}} containers</q-item-label>
        </q-item-section>

        <q-item-section side>
          <q-icon name="chat_bubble" color="green" />
        </q-item-section>
      </q-item>
    </q-list>
  </div>
</template>

<script lang="ts">
import { Vue } from 'vue-class-component'

export default class AppList extends Vue {
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
