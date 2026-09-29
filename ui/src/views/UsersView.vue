<template>
  <div class="container-fluid px-4 py-8 max-w-[95%] mx-auto">
    <div class="card bg-base-100 shadow-xl">
      <div class="card-body">
        <h2 class="card-title mb-4">Users</h2>
        <p v-if="errorMessage" role="alert" class="alert alert-error">
          {{ errorMessage }}
        </p>
        <p v-if="loading" role="status">Loading users…</p>

        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>Email</th>
                <th>Name</th>
                <th>Last Login</th>
                <th>Status</th>
                <th class="w-32">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="user in users" :key="user.id">
                <td>{{ user.email }}</td>
                <td>{{ user.name || '-' }}</td>
                <td class="whitespace-nowrap">
                  {{ user.last_login_at ? formatDate(user.last_login_at) : 'Never' }}
                </td>
                <td>
                  <span class="badge" :class="user.disabled_at ? 'badge-error' : 'badge-success'">
                    {{ user.disabled_at ? 'Disabled' : 'Active' }}
                  </span>
                </td>
                <td>
                  <button
                    v-if="user.disabled_at"
                    class="btn btn-sm"
                    @click="toggle(user, false)"
                  >
                    Enable
                  </button>
                  <button
                    v-else-if="user.id !== currentUser?.id"
                    class="btn btn-sm btn-error"
                    @click="toggle(user, true)"
                  >
                    Disable
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getMe, getUsers, setUserDisabled, formatDate, type User } from '../api'

const users = ref<User[]>([])
const currentUser = ref<User | null>(null)
const errorMessage = ref('')
const loading = ref(false)

async function fetchUsers() {
  loading.value = true
  try {
    users.value = await getUsers()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Failed to load users'
  } finally {
    loading.value = false
  }
}

async function toggle(user: User, disabled: boolean) {
  if (disabled && !confirm(`Disable ${user.email}? Their sessions and API tokens stop working immediately.`)) return
  errorMessage.value = ''
  try {
    await setUserDisabled(user.id, disabled)
    await fetchUsers()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Failed to update user'
  }
}

onMounted(async () => {
  try {
    currentUser.value = await getMe()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Failed to load current user'
  }
  await fetchUsers()
})
</script>
