<template>
  <div class="container-fluid px-4 py-8 max-w-[95%] mx-auto">
    <div class="card bg-base-100 shadow-xl">
      <div class="card-body">
        <h2 class="card-title mb-4">URL Dashboard</h2>
        <p v-if="errorMessage" role="alert" class="alert alert-error">
          {{ errorMessage }}
        </p>
        <p v-if="loading" role="status">Loading links…</p>

        <div class="flex justify-between items-center mb-4">
          <div class="form-control">
            <input
              type="text"
              aria-label="Filter links on this page"
              placeholder="Filter this page..."
              class="input input-bordered w-64"
              v-model="searchQuery"
            />
          </div>
          <div class="form-control">
            <select
              class="select select-bordered"
              aria-label="Links per page"
              v-model="perPage"
              @change="handlePerPageChange"
            >
              <option :value="20">20 per page</option>
              <option :value="50">50 per page</option>
              <option :value="100">100 per page</option>
              <option :value="500">500 per page</option>
              <option :value="1000">1000 per page</option>
            </select>
          </div>
        </div>

        <!-- URLs Table -->
        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th class="w-24">Short Code</th>
                <th class="w-1/4">Original URL</th>
                <th class="w-32">Title</th>
                <th class="w-1/3">Device URLs</th>
                <th class="w-40">Created At</th>
                <th class="w-40">Expires At</th>
                <th class="w-24">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="url in filteredUrls" :key="url.short_code">
                <td class="whitespace-nowrap">{{ url.short_code }}</td>
                <td class="break-all">
                  <a :href="url.url" target="_blank" class="link">{{
                    url.url
                  }}</a>
                </td>
                <td>{{ url.title || '-' }}</td>
                <td>
                  <div v-if="url.device_urls" class="space-y-2">
                    <div v-if="url.device_urls.android" class="text-xs">
                      <span class="font-medium">Android:</span>
                      <a
                        :href="url.device_urls.android.url"
                        target="_blank"
                        class="link break-all"
                      >
                        {{ url.device_urls.android.url }}
                      </a>
                    </div>
                    <div v-if="url.device_urls.ios" class="text-xs">
                      <span class="font-medium">iOS:</span>
                      <a
                        :href="url.device_urls.ios.url"
                        target="_blank"
                        class="link break-all"
                      >
                        {{ url.device_urls.ios.url }}
                      </a>
                    </div>
                    <div class="text-xs">
                      <span class="font-medium">Web:</span>
                      <a
                        :href="url.device_urls.web?.url || url.url"
                        target="_blank"
                        class="link break-all"
                      >
                        {{ url.device_urls.web?.url || url.url }}
                      </a>
                    </div>
                  </div>
                  <span v-else>-</span>
                </td>
                <td class="whitespace-nowrap">
                  {{ formatDate(url.created_at) }}
                </td>
                <td class="whitespace-nowrap">
                  {{ url.expires_at ? formatDate(url.expires_at) : 'Never' }}
                </td>
                <td class="whitespace-nowrap">
                  <div class="flex gap-2">
                    <button
                      class="btn btn-sm"
                      aria-label="Copy short URL"
                      @click="copyShortUrl(url.short_code)"
                    >
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                        />
                      </svg>
                    </button>
                    <button
                      class="btn btn-sm btn-warning"
                      aria-label="Edit link"
                      @click="editUrl(url)"
                    >
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z"
                        />
                      </svg>
                    </button>
                    <button
                      class="btn btn-sm btn-error"
                      aria-label="Delete link"
                      @click="deleteUrl(url.short_code)"
                    >
                      <svg
                        xmlns="http://www.w3.org/2000/svg"
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          stroke-width="2"
                          d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"
                        />
                      </svg>
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Pagination -->
        <div class="flex justify-between items-center mt-6">
          <div class="text-sm text-base-content/70">
            Showing
            {{ filteredUrls.length ? (currentPage - 1) * perPage + 1 : 0 }} to
            {{ Math.min(currentPage * perPage, totalUrls) }} of
            {{ totalUrls }} entries
          </div>
          <div class="join">
            <button
              class="join-item btn"
              :class="{ 'btn-disabled': currentPage === 1 }"
              :disabled="currentPage === 1"
              @click="changePage(currentPage - 1)"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M15 19l-7-7 7-7"
                />
              </svg>
              Previous
            </button>
            <button
              class="join-item btn"
              :class="{ 'btn-disabled': currentPage * perPage >= totalUrls }"
              :disabled="currentPage * perPage >= totalUrls"
              @click="changePage(currentPage + 1)"
            >
              Next
              <svg
                xmlns="http://www.w3.org/2000/svg"
                class="h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 5l7 7-7 7"
                />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
  <!-- Edit URL Modal -->
  <dialog id="edit_modal" class="modal">
    <div class="modal-box">
      <h3 class="font-bold text-lg mb-4">Edit URL</h3>
      <p v-if="errorMessage" role="alert" class="alert alert-error">
        {{ errorMessage }}
      </p>
      <form @submit.prevent="updateUrl">
        <div class="form-control mb-4">
          <label class="label">
            <span class="label-text">URL</span>
          </label>
          <input
            type="url"
            v-model="editingUrl.url"
            class="input input-bordered"
            required
          />
        </div>

        <div class="form-control mb-4">
          <label class="label">
            <span class="label-text">Title</span>
          </label>
          <input
            type="text"
            v-model="editingUrl.title"
            class="input input-bordered"
          />
        </div>

        <div class="form-control mb-4">
          <label class="label">
            <span class="label-text">Device URLs</span>
          </label>
          <div class="space-y-2">
            <input
              type="url"
              v-model="editingUrl.device_urls.android"
              class="input input-bordered w-full"
              placeholder="Android URL"
            />
            <input
              type="url"
              v-model="editingUrl.device_urls.ios"
              class="input input-bordered w-full"
              placeholder="iOS URL"
            />
            <input
              type="url"
              v-model="editingUrl.device_urls.web"
              class="input input-bordered w-full"
              placeholder="Web URL (optional)"
            />
            <p class="text-sm">
              Unrecognized devices use Web. Missing platform destinations use
              the original URL. Use ?platform=ios or ?platform=android when the
              device is known.
            </p>
          </div>
        </div>

        <div class="modal-action">
          <button type="submit" class="btn btn-primary">Save Changes</button>
          <button type="button" class="btn" @click="closeEditModal">
            Cancel
          </button>
        </div>
      </form>
    </div>
    <form method="dialog" class="modal-backdrop">
      <button>close</button>
    </form>
  </dialog>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { request, type ShortURL, type URLEdit } from '../api'

const urls = ref<ShortURL[]>([])
const errorMessage = ref('')
const publicURL = ref('')
const editingUrl = ref<URLEdit>({
  short_code: '',
  url: '',
  title: '',
  device_urls: {
    android: '',
    ios: '',
    web: '',
  },
})
const searchQuery = ref('')
const currentPage = ref(1)
const perPage = ref(20)
const totalUrls = ref(0)
const loading = ref(false)

const filteredUrls = computed(() => {
  if (!searchQuery.value) return urls.value

  const query = searchQuery.value.toLowerCase()
  return urls.value.filter(
    url =>
      url.short_code.toLowerCase().includes(query) ||
      url.url.toLowerCase().includes(query) ||
      (url.title && url.title.toLowerCase().includes(query)),
  )
})

async function fetchUrls(page = 1) {
  loading.value = true
  errorMessage.value = ''
  try {
    const response = await request(
      `/api/v1/urls?page=${page}&per_page=${perPage.value}`,
    )
    const data: {
      status: string
      data: { urls: ShortURL[]; count: number; page: number }
    } = await response.json()

    if (data.status === 'success') {
      urls.value = data.data.urls
      totalUrls.value = data.data.count
      currentPage.value = data.data.page
    }
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : 'Failed to load links'
  } finally {
    loading.value = false
  }
}

function changePage(page: number) {
  // Calculate total pages
  const totalPages = Math.ceil(totalUrls.value / perPage.value)

  // Validate page number
  if (page < 1 || page > totalPages) return

  currentPage.value = page
  fetchUrls(page)
}

function handlePerPageChange() {
  currentPage.value = 1 // Reset to first page when changing items per page
  fetchUrls(1)
}

function formatDate(dateString: string) {
  return new Date(dateString).toLocaleString()
}

async function copyShortUrl(shortCode: string) {
  try {
    if (!publicURL.value) throw new Error('Public URL is not configured')
    const url = `${publicURL.value.replace(/\/$/, '')}/${encodeURIComponent(shortCode)}`
    await navigator.clipboard.writeText(url)
    // Show success toast
    const toast = document.createElement('div')
    toast.className = 'toast toast-end'
    toast.innerHTML = `
      <div class="alert alert-success">
        <span>URL copied to clipboard!</span>
      </div>
    `
    document.body.appendChild(toast)
    setTimeout(() => {
      toast.remove()
    }, 3000)
  } catch (err) {
    console.error('Failed to copy URL:', err)
    // Show error toast
    const toast = document.createElement('div')
    toast.className = 'toast toast-end'
    toast.innerHTML = `
      <div class="alert alert-error">
        <span>Failed to copy URL</span>
      </div>
    `
    document.body.appendChild(toast)
    setTimeout(() => {
      toast.remove()
    }, 3000)
  }
}

function editUrl(url: ShortURL) {
  errorMessage.value = ''
  editingUrl.value = {
    short_code: url.short_code,
    url: url.url,
    title: url.title || '',
    device_urls: {
      android: url.device_urls?.android?.url || '',
      ios: url.device_urls?.ios?.url || '',
      web: url.device_urls?.web?.url || '',
    },
  }
  const modal = document.getElementById('edit_modal')
  if (modal instanceof HTMLDialogElement) modal.showModal()
}

function closeEditModal() {
  const modal = document.getElementById('edit_modal')
  if (modal instanceof HTMLDialogElement) modal.close()
}

async function updateUrl() {
  try {
    const response = await request(
      `/api/v1/urls/${encodeURIComponent(editingUrl.value.short_code)}`,
      {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          url: editingUrl.value.url,
          title: editingUrl.value.title,
          device_urls: {
            android: editingUrl.value.device_urls.android,
            ios: editingUrl.value.device_urls.ios,
            web: editingUrl.value.device_urls.web,
          },
        }),
      },
    )

    if (response.ok) {
      closeEditModal()
      fetchUrls(currentPage.value)
    } else {
      console.error('Failed to update URL')
    }
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : 'Failed to update link'
  }
}

async function deleteUrl(shortCode: string) {
  if (!confirm('Are you sure you want to delete this URL?')) {
    return
  }

  try {
    const response = await request(
      `/api/v1/urls/${encodeURIComponent(shortCode)}`,
      {
        method: 'DELETE',
      },
    )
    if (response.status === 204) {
      // Refresh the current page
      fetchUrls(currentPage.value)
    }
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : 'Failed to delete link'
  }
}

onMounted(async () => {
  fetchUrls()
  try {
    const response = await request('/api/v1')
    const body: { data: { public_url: string } } = await response.json()
    publicURL.value = body.data.public_url
  } catch (error) {
    errorMessage.value =
      error instanceof Error ? error.message : 'Failed to load configuration'
  }
})
</script>
