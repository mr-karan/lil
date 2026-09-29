<template>
  <div class="container-fluid px-4 py-8 max-w-[95%] mx-auto">
    <div class="card bg-base-100 shadow-xl">
      <div class="card-body">
        <h2 class="card-title mb-4">Activity</h2>
        <p v-if="errorMessage" role="alert" class="alert alert-error">
          {{ errorMessage }}
        </p>
        <p v-if="loading" role="status">Loading activity…</p>

        <div v-if="shortCode" class="flex items-center gap-2 mb-4">
          <span class="badge badge-info">Link: {{ shortCode }}</span>
          <a href="/admin/activity" class="btn btn-sm">Clear filter</a>
        </div>

        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th class="w-40">Time</th>
                <th class="w-40">Actor</th>
                <th class="w-32">Action</th>
                <th class="w-32">Target</th>
                <th>Changes</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="entry in entries" :key="entry.id">
                <td class="whitespace-nowrap">{{ formatDate(entry.created_at) }}</td>
                <td class="whitespace-nowrap">
                  {{ displayName(entry.actor) }}
                  <span v-if="entry.token_id !== null" class="badge badge-ghost">API token</span>
                </td>
                <td class="whitespace-nowrap">{{ entry.action }}</td>
                <td class="break-all">{{ entry.target }}</td>
                <td>
                  <details v-if="entry.before != null || entry.after != null">
                    <summary class="cursor-pointer text-sm">Show diff</summary>
                    <div class="grid grid-cols-2 gap-2 mt-2">
                      <div>
                        <div class="text-xs font-medium">Before</div>
                        <pre class="text-xs whitespace-pre-wrap break-all">{{ pretty(entry.before) }}</pre>
                      </div>
                      <div>
                        <div class="text-xs font-medium">After</div>
                        <pre class="text-xs whitespace-pre-wrap break-all">{{ pretty(entry.after) }}</pre>
                      </div>
                    </div>
                  </details>
                  <span v-else>-</span>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="flex justify-between items-center mt-6">
          <div class="text-sm text-base-content/70">
            Showing {{ entries.length ? (page - 1) * perPage + 1 : 0 }} to
            {{ Math.min(page * perPage, count) }} of {{ count }} entries
          </div>
          <div class="join">
            <button class="join-item btn" :disabled="page === 1" @click="fetchAudit(page - 1)">
              Previous
            </button>
            <button class="join-item btn" :disabled="page * perPage >= count" @click="fetchAudit(page + 1)">
              Next
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getAudit, displayName, formatDate, type AuditEntry } from '../api'

const shortCode = new URLSearchParams(window.location.search).get('short_code') ?? ''
const entries = ref<AuditEntry[]>([])
const errorMessage = ref('')
const loading = ref(false)
const page = ref(1)
const perPage = ref(50)
const count = ref(0)

function pretty(value: unknown): string {
  return value == null ? '-' : JSON.stringify(value, null, 2)
}

async function fetchAudit(requestedPage = 1) {
  loading.value = true
  errorMessage.value = ''
  try {
    const query = new URLSearchParams({ page: String(requestedPage), per_page: String(perPage.value) })
    if (shortCode) query.set('short_code', shortCode)
    const data = await getAudit(query)
    entries.value = data.entries
    page.value = data.page
    perPage.value = data.per_page
    count.value = data.count
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Failed to load activity'
  } finally {
    loading.value = false
  }
}

onMounted(() => fetchAudit())
</script>
