<template>
  <main class="app-shell">
    <section v-if="!authenticated" class="login-screen">
          <n-card class="login-card" title="OpenList STRM">
            <n-form :model="loginForm" @submit.prevent="login">
              <n-form-item label="用户名" :label-props="{ for: 'login-username' }">
                <n-input v-model:value="loginForm.username" input-id="login-username" name="username" autocomplete="username" :input-props="{ id: 'login-username', name: 'username', autocomplete: 'username', 'aria-label': '用户名' }" />
              </n-form-item>
              <n-form-item label="密码" :label-props="{ for: 'login-password' }">
                <n-input v-model:value="loginForm.password" input-id="login-password" name="password" type="password" autocomplete="current-password" :input-props="{ id: 'login-password', name: 'password', autocomplete: 'current-password', 'aria-label': '密码' }" show-password-on="click" />
              </n-form-item>
              <n-button type="primary" block :loading="loading" @click="login">登录</n-button>
            </n-form>
          </n-card>
        </section>

    <n-layout v-else class="workspace">
      <n-layout-header class="topbar">
            <div>
              <h1>OpenList STRM</h1>
              <p>定时生成 Emby STRM 与本地镜像文件</p>
            </div>
            <n-space align="center">
              <n-tag v-if="activeRuns.length" type="info">运行中 {{ activeRuns.length }}</n-tag>
              <n-text>{{ username }}</n-text>
              <n-button secondary @click="logout">退出</n-button>
            </n-space>
          </n-layout-header>

      <n-layout-content class="content">
            <n-grid :cols="12" :x-gap="16" :y-gap="16" responsive="screen">
              <n-gi :span="12" :l="4">
                <n-card title="OpenList 设置" size="small">
                  <n-form :model="settingsForm" label-placement="top">
                    <n-form-item label="OpenList API 地址" :label-props="{ for: 'openlist-api-base-url' }">
                      <n-input v-model:value="settingsForm.base_url" input-id="openlist-api-base-url" name="openlist_api_base_url" autocomplete="url" :input-props="{ id: 'openlist-api-base-url', name: 'openlist_api_base_url', autocomplete: 'url', 'aria-label': 'OpenList API 地址' }" placeholder="http://openlist:5244" />
                    </n-form-item>
                    <n-form-item label="STRM 下载地址" :label-props="{ for: 'openlist-download-base-url' }">
                      <n-input v-model:value="settingsForm.download_base_url" input-id="openlist-download-base-url" name="openlist_download_base_url" autocomplete="url" :input-props="{ id: 'openlist-download-base-url', name: 'openlist_download_base_url', autocomplete: 'url', 'aria-label': 'STRM 下载地址' }" placeholder="https://openlist.example.com" />
                    </n-form-item>
                    <n-form-item label="用户名" :label-props="{ for: 'openlist-username' }">
                      <n-input v-model:value="settingsForm.username" input-id="openlist-username" name="openlist_username" autocomplete="username" :input-props="{ id: 'openlist-username', name: 'openlist_username', autocomplete: 'username', 'aria-label': 'OpenList 用户名' }" />
                    </n-form-item>
                    <n-form-item :label="settings?.password_set ? '密码（留空保持不变）' : '密码'" :label-props="{ for: 'openlist-password' }">
                      <n-input v-model:value="settingsForm.password" input-id="openlist-password" name="openlist_password" type="password" autocomplete="new-password" :input-props="{ id: 'openlist-password', name: 'openlist_password', autocomplete: 'new-password', 'aria-label': 'OpenList 密码' }" show-password-on="click" />
                    </n-form-item>
                    <n-space>
                      <n-button type="primary" :loading="savingSettings" @click="saveSettings">保存</n-button>
                      <n-button :loading="testingSettings" @click="testSettings">测试登录</n-button>
                    </n-space>
                  </n-form>
                </n-card>

                <n-card title="当前扫描" size="small" class="mt">
                  <n-empty v-if="!activeRuns.length" description="没有运行中的任务" />
                  <n-space v-else vertical>
                    <n-thing v-for="run in activeRuns" :key="run.run_id" :title="run.task_name" :description="formatTime(run.started_at)">
                      <template #action>
                        <n-button size="small" tertiary type="warning" @click="stopTask(run.task_id)">停止</n-button>
                      </template>
                    </n-thing>
                  </n-space>
                </n-card>
              </n-gi>

              <n-gi :span="12" :l="8">
                <n-card title="任务" size="small">
                  <template #header-extra>
                    <n-space>
                      <n-button secondary @click="refreshAll">刷新</n-button>
                      <n-button type="primary" @click="openTask()">新建任务</n-button>
                    </n-space>
                  </template>
                  <n-data-table :columns="taskColumns" :data="tasks" :loading="loading" :row-key="(row: Task) => row.id" />
                </n-card>

                <n-card title="运行记录" size="small" class="mt">
                  <n-data-table :columns="runColumns" :data="runs" :loading="loading" :row-key="(row: Run) => row.id" />
                </n-card>
              </n-gi>
            </n-grid>
          </n-layout-content>
        </n-layout>

    <n-drawer v-model:show="taskDrawer" width="720" placement="right">
      <n-drawer-content :title="taskForm.id ? '编辑任务' : '新建任务'" closable>
            <n-form :model="taskForm" label-placement="top">
              <n-grid :cols="2" :x-gap="16">
                <n-gi>
                  <n-form-item label="名称" :label-props="{ for: 'task-name' }">
                    <n-input v-model:value="taskForm.name" input-id="task-name" name="task_name" autocomplete="off" :input-props="{ id: 'task-name', name: 'task_name', autocomplete: 'off', 'aria-label': '名称' }" />
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item label="Cron" :label-props="{ for: 'task-cron' }">
                    <n-input v-model:value="taskForm.cron" input-id="task-cron" name="task_cron" autocomplete="off" :input-props="{ id: 'task-cron', name: 'task_cron', autocomplete: 'off', 'aria-label': 'Cron' }" placeholder="0 3 * * *" />
                  </n-form-item>
                </n-gi>
              </n-grid>
              <n-form-item label="输出根目录" :label-props="{ for: 'task-output-root' }">
                <n-input v-model:value="taskForm.output_root" input-id="task-output-root" name="task_output_root" autocomplete="off" :input-props="{ id: 'task-output-root', name: 'task_output_root', autocomplete: 'off', 'aria-label': '输出根目录' }" placeholder="/media" />
              </n-form-item>
              <n-grid :cols="2" :x-gap="16">
                <n-gi>
                  <n-form-item :show-label="false">
                    <div class="field-block">
                      <span class="field-label">启用</span>
                      <n-switch v-model:value="taskForm.enabled" aria-label="启用" />
                    </div>
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item :show-label="false">
                    <div class="field-block">
                      <span class="field-label">同步模式</span>
                      <n-select v-model:value="taskForm.sync_mode" :options="syncOptions" aria-label="同步模式" />
                    </div>
                  </n-form-item>
                </n-gi>
              </n-grid>
              <n-grid :cols="2" :x-gap="16">
                <n-gi>
                  <n-form-item :show-label="false">
                    <div class="field-block">
                      <span class="field-label">URL 编码</span>
                      <n-switch v-model:value="taskForm.encode_url" aria-label="URL 编码" />
                    </div>
                  </n-form-item>
                </n-gi>
                <n-gi>
                  <n-form-item label="下载并发" :label-props="{ for: 'task-download-concurrency' }">
                    <n-input-number v-model:value="taskForm.download_concurrency" :input-props="{ id: 'task-download-concurrency', name: 'task_download_concurrency', autocomplete: 'off', 'aria-label': '下载并发' }" :min="1" :max="16" class="full" />
                  </n-form-item>
                </n-gi>
              </n-grid>
              <n-form-item label="下载超时（秒）" :label-props="{ for: 'task-download-timeout' }">
                <n-input-number v-model:value="taskForm.download_timeout_seconds" :input-props="{ id: 'task-download-timeout', name: 'task_download_timeout', autocomplete: 'off', 'aria-label': '下载超时' }" :min="1" class="full" />
              </n-form-item>
              <n-form-item :show-label="false">
                <div class="field-block">
                  <span class="field-label">扫描目录</span>
                  <n-dynamic-tags v-model:value="taskForm.scan_dirs" />
                </div>
              </n-form-item>
              <n-form-item :show-label="false">
                <div class="field-block">
                  <span class="field-label">生成 STRM 的后缀</span>
                  <n-dynamic-tags v-model:value="taskForm.strm_extensions" />
                </div>
              </n-form-item>
              <n-form-item :show-label="false">
                <div class="field-block">
                  <span class="field-label">直接下载的后缀</span>
                  <n-dynamic-tags v-model:value="taskForm.download_extensions" />
                </div>
              </n-form-item>
              <n-form-item :show-label="false">
                <div class="field-block">
                  <span class="field-label">黑名单 Glob</span>
                  <n-dynamic-tags v-model:value="taskForm.blacklist" />
                </div>
              </n-form-item>
              <n-alert v-if="taskError" type="error" class="mb">{{ taskError }}</n-alert>
              <n-space justify="end">
                <n-button @click="taskDrawer = false">取消</n-button>
                <n-button type="primary" :loading="savingTask" @click="saveTask">保存任务</n-button>
              </n-space>
            </n-form>
          </n-drawer-content>
        </n-drawer>

    <n-modal v-model:show="runModal" preset="card" title="运行详情" class="run-modal">
          <n-spin :show="loadingRunDetail">
            <template v-if="runDetail">
              <n-descriptions :column="2" bordered size="small">
                <n-descriptions-item label="任务">{{ runDetail.run.task_name || runDetail.run.task_id }}</n-descriptions-item>
                <n-descriptions-item label="状态"><n-tag :type="runTagType(runDetail.run.status)">{{ runDetail.run.status }}</n-tag></n-descriptions-item>
                <n-descriptions-item label="开始">{{ formatTime(runDetail.run.started_at) }}</n-descriptions-item>
                <n-descriptions-item label="结束">{{ runDetail.run.ended_at ? formatTime(runDetail.run.ended_at) : '-' }}</n-descriptions-item>
              </n-descriptions>
              <n-divider />
              <n-log :log="eventLog" language="text" :rows="14" />
            </template>
          </n-spin>
        </n-modal>
  </main>
</template>

<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue'
import type { DataTableColumns } from 'naive-ui'
import { NButton, NPopconfirm, NSpace, NTag, useMessage } from 'naive-ui'
import { api, defaultTask, type ActiveRun, type ApiError, type OpenListSettings, type Run, type RunEvent, type Task } from './api'

const message = useMessage()
const authenticated = ref(false)
const username = ref('')
const loading = ref(false)
const savingSettings = ref(false)
const testingSettings = ref(false)
const savingTask = ref(false)
const taskDrawer = ref(false)
const runModal = ref(false)
const loadingRunDetail = ref(false)
const taskError = ref('')
const settings = ref<OpenListSettings | null>(null)
const tasks = ref<Task[]>([])
const runs = ref<Run[]>([])
const activeRuns = ref<ActiveRun[]>([])
const runDetail = ref<{ run: Run; events: RunEvent[] } | null>(null)

const loginForm = reactive({ username: '', password: '' })
const settingsForm = reactive({ base_url: '', download_base_url: '', username: '', password: '' })
const taskForm = reactive<Task>(defaultTask())
const syncOptions = [
  { label: '宽松同步', value: 'loose' },
  { label: '严格同步', value: 'strict' }
]

const eventLog = computed(() => {
  if (!runDetail.value) return ''
  return runDetail.value.events.map((ev) => `[${formatTime(ev.created_at)}] ${ev.level.toUpperCase()} ${ev.message}`).join('\n')
})

const taskColumns: DataTableColumns<Task> = [
  { title: '名称', key: 'name', minWidth: 140 },
  { title: 'Cron', key: 'cron', width: 120 },
  { title: '输出目录', key: 'output_root', minWidth: 180, ellipsis: { tooltip: true } },
  {
    title: '模式',
    key: 'sync_mode',
    width: 96,
    render(row) {
      return h(NTag, { type: row.sync_mode === 'strict' ? 'warning' : 'success', size: 'small' }, { default: () => row.sync_mode === 'strict' ? '严格' : '宽松' })
    }
  },
  {
    title: '启用',
    key: 'enabled',
    width: 80,
    render(row) {
      return h(NTag, { type: row.enabled ? 'success' : 'default', size: 'small' }, { default: () => row.enabled ? '启用' : '停用' })
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 260,
    render(row) {
      return h(NSpace, { size: 8 }, () => [
        h(NButton, { size: 'small', secondary: true, onClick: () => openTask(row) }, { default: () => '编辑' }),
        h(NButton, { size: 'small', type: 'primary', secondary: true, onClick: () => runTask(row.id) }, { default: () => '运行' }),
        h(NButton, { size: 'small', type: 'warning', secondary: true, onClick: () => stopTask(row.id) }, { default: () => '停止' }),
        h(NPopconfirm, { onPositiveClick: () => deleteTask(row.id) }, {
          trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true }, { default: () => '删除' }),
          default: () => '删除这个任务？'
        })
      ])
    }
  }
]

const runColumns: DataTableColumns<Run> = [
  { title: 'ID', key: 'id', width: 70 },
  { title: '任务', key: 'task_name', minWidth: 140 },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render(row) {
      return h(NTag, { type: runTagType(row.status), size: 'small' }, { default: () => row.status })
    }
  },
  { title: '触发', key: 'trigger', width: 90 },
  {
    title: '统计',
    key: 'stats',
    minWidth: 220,
    render(row) {
      return `文件 ${row.stats.files} / STRM ${row.stats.strm_written} / 下载 ${row.stats.downloads} / 删除 ${row.stats.deleted} / 错误 ${row.stats.errors}`
    }
  },
  {
    title: '开始时间',
    key: 'started_at',
    width: 170,
    render(row) { return formatTime(row.started_at) }
  },
  {
    title: '操作',
    key: 'actions',
    width: 90,
    render(row) {
      return h(NButton, { size: 'small', secondary: true, onClick: () => openRun(row.id) }, { default: () => '详情' })
    }
  }
]

onMounted(async () => {
  try {
    const session = await api.session()
    authenticated.value = session.authenticated
    username.value = session.username || ''
    if (session.authenticated) {
      await refreshAll()
    }
  } catch {
    authenticated.value = false
  }
})

async function login() {
  loading.value = true
  try {
    const me = await api.login(loginForm.username, loginForm.password)
    username.value = me.username
    authenticated.value = true
    await refreshAll()
  } catch (err) {
    showError(err)
  } finally {
    loading.value = false
  }
}

async function logout() {
  await api.logout().catch(() => undefined)
  authenticated.value = false
}

async function refreshAll() {
  loading.value = true
  try {
    const [settingsData, tasksData, runsData, statusData] = await Promise.all([
      api.getSettings(),
      api.listTasks(),
      api.listRuns(),
      api.status()
    ])
    settings.value = settingsData
    settingsForm.base_url = settingsData.base_url
    settingsForm.download_base_url = settingsData.download_base_url
    settingsForm.username = settingsData.username
    settingsForm.password = ''
    tasks.value = tasksData
    runs.value = runsData
    activeRuns.value = statusData.active_runs || []
  } catch (err) {
    handleAuthOrError(err)
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  savingSettings.value = true
  try {
    settings.value = await api.saveSettings({ ...settingsForm })
    settingsForm.password = ''
    message.success('OpenList 设置已保存')
  } catch (err) {
    showError(err)
  } finally {
    savingSettings.value = false
  }
}

async function testSettings() {
  testingSettings.value = true
  try {
    await api.testSettings()
    message.success('OpenList 登录成功')
  } catch (err) {
    showError(err)
  } finally {
    testingSettings.value = false
  }
}

function openTask(task?: Task) {
  Object.assign(taskForm, task ? structuredClone(task) : defaultTask())
  taskError.value = ''
  taskDrawer.value = true
}

async function saveTask() {
  taskError.value = validateTask(taskForm)
  if (taskError.value) return
  savingTask.value = true
  try {
    const saved = taskForm.id ? await api.updateTask(taskForm) : await api.createTask(taskForm)
    message.success(`任务 ${saved.name} 已保存`)
    taskDrawer.value = false
    await refreshAll()
  } catch (err) {
    showError(err)
  } finally {
    savingTask.value = false
  }
}

async function deleteTask(id: number) {
  try {
    await api.deleteTask(id)
    message.success('任务已删除')
    await refreshAll()
  } catch (err) {
    showError(err)
  }
}

async function runTask(id: number) {
  try {
    await api.runTask(id)
    message.success('扫描已开始')
    await refreshAll()
  } catch (err) {
    showError(err)
  }
}

async function stopTask(id: number) {
  try {
    const res = await api.stopTask(id)
    message[res.stopped ? 'success' : 'warning'](res.stopped ? '已发送停止请求' : '任务未在运行')
    await refreshAll()
  } catch (err) {
    showError(err)
  }
}

async function openRun(id: number) {
  runModal.value = true
  loadingRunDetail.value = true
  try {
    runDetail.value = await api.getRun(id)
  } catch (err) {
    showError(err)
  } finally {
    loadingRunDetail.value = false
  }
}

function validateTask(task: Task) {
  if (!task.name.trim()) return '任务名称不能为空'
  if (!task.cron.trim()) return 'Cron 不能为空'
  if (!task.output_root.startsWith('/') || task.output_root === '/') return '输出根目录必须是非根的绝对路径'
  if (!task.scan_dirs.length) return '至少配置一个扫描目录'
  if (task.download_concurrency < 1 || task.download_concurrency > 16) return '下载并发必须在 1 到 16 之间'
  if (task.download_timeout_seconds < 1) return '下载超时必须大于 0'
  return ''
}

function runTagType(status: string): 'default' | 'success' | 'warning' | 'error' | 'info' {
  if (status === 'success') return 'success'
  if (status === 'running') return 'info'
  if (status === 'canceled') return 'warning'
  if (status === 'failed') return 'error'
  return 'default'
}

function handleAuthOrError(err: unknown) {
  const apiErr = err as ApiError
  if (apiErr.status === 401) {
    authenticated.value = false
    return
  }
  showError(err)
}

function showError(err: unknown) {
  message.error((err as Error).message || '请求失败')
}

function formatTime(value: string) {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}
</script>
