<template>
  <div class="container-fluid px-4 py-8 max-w-[95%] mx-auto">
    <div class="card bg-base-100 shadow-xl">
      <div class="card-body">
        <h2 class="card-title mb-4">API Tokens</h2>
        <p v-if="errorMessage" role="alert" class="alert alert-error">
          {{ errorMessage }}
        </p>
        <p v-if="loading" role="status">Loading tokens…</p>

        <div v-if="newToken" role="alert" class="alert alert-success flex-col items-start">
          <span>This token will not be shown again. Copy it now.</span>
          <div class="flex items-center gap-2 w-full">
            <code class="break-all">{{ newToken }}</code>
            <button class="btn btn-sm" @click="copyToken">{{ copied ? 'Copied' : 'Copy' }}</button>
          </div>
        </div>

        <form class="flex gap-2 mb-4" @submit.prevent="addToken">
          <input
            type="text"
            v-model="newName"
            aria-label="Token name"
            placeholder="Token name"
            maxlength="64"
            class="input input-bordered w-64"
            required
          />
          <button type="submit" class="btn btn-primary">Create token</button>
        </form>

        <div class="overflow-x-auto">
          <table class="table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Prefix</th>
                <th>Created</th>
                <th class="w-24">Actions</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="token in tokens" :key="token.id">
                <td>{{ token.name }}</td>
                <td><code>{{ token.token_prefix }}…</code></td>
                <td class="whitespace-nowrap">{{ formatDate(token.created_at) }}</td>
                <td>
                  <button class="btn btn-sm btn-error" @click="revoke(token)">Revoke</button>
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
import { getTokens, createToken, revokeToken, formatDate, type APIToken } from '../api'

const tokens = ref<APIToken[]>([])
const errorMessage = ref('')
const loading = ref(false)
const newName = ref('')
const newToken = ref('')
const copied = ref(false)

async function fetchTokens() {
  loading.value = true
  try {
    tokens.value = await getTokens()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Failed to load tokens'
  } finally {
    loading.value = false
  }
}

async function addToken() {
  errorMessage.value = ''
  try {
    const created = await createToken(newName.value.trim())
    newToken.value = created.token
    copied.value = false
    newName.value = ''
    await fetchTokens()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Failed to create token'
  }
}

async function copyToken() {
  try {
    await navigator.clipboard.writeText(newToken.value)
    copied.value = true
  } catch {
    errorMessage.value = 'Failed to copy token'
  }
}

async function revoke(token: APIToken) {
  if (!confirm(`Revoke token "${token.name}"? Scripts using it will stop working.`)) return
  errorMessage.value = ''
  try {
    await revokeToken(token.id)
    await fetchTokens()
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'Failed to revoke token'
  }
}

onMounted(fetchTokens)
</script>
