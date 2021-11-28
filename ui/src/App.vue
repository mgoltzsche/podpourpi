<template>
  <q-layout view="lHh Lpr lFf">
    <q-header elevated class="glossy">
      <q-toolbar class="bg-primary text-white">
        <q-btn
          flat
          dense
          round
          @click="leftDrawerOpen = !leftDrawerOpen"
          aria-label="Menu"
          icon="menu"
        />

        <q-toolbar-title>
          Podpourpi {{ $route.name }}
        </q-toolbar-title>

        <div>Quasar v{{ $q.version }}</div>
      </q-toolbar>
    </q-header>

    <q-drawer
      v-model="leftDrawerOpen"
      show-if-above
      bordered
      class="bg-grey-2"
    >
      <q-list>
        <q-item-label header>podpourpi</q-item-label>
        <q-item to="/" exact>
          <q-item-section avatar>
            <q-icon name="home" />
          </q-item-section>
          <q-item-section>
            <q-item-label>Home</q-item-label>
            <q-item-label caption>Overview</q-item-label>
          </q-item-section>
        </q-item>
        <q-item to="/apps">
          <q-item-section avatar>
            <q-icon name="apps" />
          </q-item-section>
          <q-item-section>
            <q-item-label>Apps</q-item-label>
            <q-item-label caption>Host-local applications</q-item-label>
          </q-item-section>
        </q-item>
        <q-item to="/nodes">
          <q-item-section avatar>
            <q-icon name="cable" />
          </q-item-section>
          <q-item-section>
            <q-item-label>Nodes</q-item-label>
            <q-item-label caption>Other podpourpi hosts</q-item-label>
          </q-item-section>
        </q-item>
        <q-item to="/wifi">
          <q-item-section avatar>
            <q-icon name="wifi" />
          </q-item-section>
          <q-item-section>
            <q-item-label>Wifi</q-item-label>
            <q-item-label caption>Select wifi network to connect to</q-item-label>
          </q-item-section>
        </q-item>
        <q-item to="/about">
          <q-item-section avatar>
            <q-icon name="info" />
          </q-item-section>
          <q-item-section>
            <q-item-label>About</q-item-label>
            <q-item-label caption>About podpourpi</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>
    </q-drawer>

    <q-page-container>
      <q-breadcrumbs v-if="$route.path != '/'" class="q-pa-md">
        <q-breadcrumbs-el icon="home" to="/" />
        <q-breadcrumbs-el v-for="r in $route.matched.slice(0, $route.matched.length-1)" :key="r.name" :label="r.name?.toString()" :to="r.path" />
        <q-breadcrumbs-el :label="$route.name?.toString()" />
      </q-breadcrumbs>
      <router-view/>
    </q-page-container>
  </q-layout>
</template>

<script lang="ts">
import { Options, Vue } from 'vue-class-component'
import { RouteLocationMatched } from 'vue-router'
import HelloWorld from './components/HelloWorld.vue'

class Breadcrumb {
  public name!: string
  public path!: string
}

@Options({
  components: {
    HelloWorld,
  },
})
export default class App extends Vue {
  leftDrawerOpen = false
  /*breadcrumbs(): Breadcrumb[] {
    let pathSegments = this.$route.path.split('/')
    pathSegments = pathSegments.slice(1, pathSegments.length-1)
    return pathSegments.reduce<Breadcrumb[]>((breadcrumbs, segment, idx) => {
      const path = `${breadcrumbs[idx-1]?.path || ''}/${segment}`
      this.$router.resolve
      breadcrumbs.push({name: segment, path: path})
      return breadcrumbs
    }, [])
  }*/
  breadcrumbs(): RouteLocationMatched[] {
    return this.$route.matched
  }
}
</script>
