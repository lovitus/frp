import { createRouter, createWebHashHistory } from 'vue-router'
import ServerOverview from '../views/ServerOverview.vue'
import Clients from '../views/Clients.vue'
import ClientDetail from '../views/ClientDetail.vue'
import Proxies from '../views/Proxies.vue'
import ProxyDetail from '../views/ProxyDetail.vue'
import GatewayTunnels from '../views/GatewayTunnels.vue'

const router = createRouter({
  history: createWebHashHistory(),
  scrollBehavior() {
    return { top: 0 }
  },
  routes: [
    {
      path: '/',
      name: 'ServerOverview',
      component: ServerOverview,
    },
    {
      path: '/clients',
      name: 'Clients',
      component: Clients,
    },
    {
      path: '/clients/:key',
      name: 'ClientDetail',
      component: ClientDetail,
    },
    {
      path: '/proxies/:type?',
      name: 'Proxies',
      component: Proxies,
    },
    {
      path: '/gateway-tunnels',
      name: 'GatewayTunnels',
      component: GatewayTunnels,
    },
    {
      path: '/proxy/:name',
      name: 'ProxyDetail',
      component: ProxyDetail,
    },
  ],
})

export default router
