import { createApp, h } from 'vue'
import {
  create,
  NAlert,
  NButton,
  NCard,
  NCheckbox,
  NConfigProvider,
  NDataTable,
  NDescriptions,
  NDescriptionsItem,
  NDivider,
  NDrawer,
  NDrawerContent,
  NDynamicTags,
  NEmpty,
  NForm,
  NFormItem,
  NGrid,
  NGi,
  NIcon,
  NInput,
  NInputNumber,
  NLayout,
  NLayoutContent,
  NLayoutHeader,
  NLog,
  NModal,
  NPopconfirm,
  NSelect,
  NSpace,
  NSpin,
  NSwitch,
  NTag,
  NText,
  NThing,
  NTimeline,
  NTimelineItem,
  NTooltip,
  NMessageProvider
} from 'naive-ui'
import App from './App.vue'
import './styles.css'

const naive = create({
  components: [
    NAlert,
    NButton,
    NCard,
    NCheckbox,
    NConfigProvider,
    NDataTable,
    NDescriptions,
    NDescriptionsItem,
    NDivider,
    NDrawer,
    NDrawerContent,
    NDynamicTags,
    NEmpty,
    NForm,
    NFormItem,
    NGrid,
    NGi,
    NIcon,
    NInput,
    NInputNumber,
    NLayout,
    NLayoutContent,
    NLayoutHeader,
    NLog,
    NModal,
    NPopconfirm,
    NSelect,
    NSpace,
    NSpin,
    NSwitch,
    NTag,
    NText,
    NThing,
    NTimeline,
    NTimelineItem,
    NTooltip,
    NMessageProvider
  ]
})

const Root = {
  render() {
    return h(NConfigProvider, null, {
      default: () =>
        h(NMessageProvider, null, {
          default: () => h(App)
        })
    })
  }
}

createApp(Root).use(naive).mount('#app')
