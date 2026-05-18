import { createApp } from 'vue'
import 'mdb-vue-ui-kit/css/mdb.min.css'
import './style.css'
import App from './App.vue'
import router from './router'

createApp(App).use(router).mount('#app')
