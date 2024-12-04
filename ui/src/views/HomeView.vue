<template>
  <div class="min-h-screen bg-base-200 p-4 md:p-6">
    <div class="max-w-3xl mx-auto pt-4">
      <!-- Simplified header -->
      <div class="mb-6 animate-fade-in">
        <h2 class="text-xl font-medium text-base-content/70">Create a new short URL</h2>
      </div>

      <!-- Main Card -->
      <div class="card bg-base-100 shadow-xl">
        <div class="card-body p-8">
          <!-- URL Input Form -->
          <form @submit.prevent="handleSubmit" class="space-y-6">
            <!-- Main URL Input with icon -->
            <div class="form-control">
              <div class="join w-full">
                <div class="join-item bg-base-200 px-4 flex items-center">
                  <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 text-base-content/70" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.828 10.172a4 4 0 00-5.656 0l-4 4a4 4 0 105.656 5.656l1.102-1.101m-.758-4.899a4 4 0 005.656 0l4-4a4 4 0 00-5.656-5.656l-1.1 1.1" />
                  </svg>
                </div>
                <input
                  type="url"
                  v-model="formData.url"
                  placeholder="Paste your long URL here"
                  class="input join-item input-bordered w-full text-lg"
                  required
                />
                <button type="submit" class="btn join-item btn-primary px-8 font-semibold">
                  <span v-if="loading" class="loading loading-spinner"></span>
                  <span v-else>Shorten</span>
                </button>
              </div>
            </div>

            <!-- Advanced Options (Collapsible) -->
            <div class="collapse collapse-arrow bg-base-200 rounded-box hover:bg-base-300 transition-colors duration-200">
              <input type="checkbox" />
              <div class="collapse-title font-medium flex items-center gap-2 select-none">
                <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6V4m0 2a2 2 0 100 4m0-4a2 2 0 110 4m-6 8a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4m6 6v10m6-2a2 2 0 100-4m0 4a2 2 0 110-4m0 4v2m0-6V4" />
                </svg>
                Advanced Options
              </div>
              <div class="collapse-content space-y-4">
                <!-- Custom Slug with helper text -->
                <div class="form-control">
                  <label class="label cursor-pointer select-text">
                    <span class="label-text font-medium">Custom Slug</span>
                    <span class="label-text-alt text-base-content/60">Letters, numbers, and hyphens only</span>
                  </label>
                  <input
                    type="text"
                    v-model="formData.slug"
                    placeholder="e.g., my-custom-url"
                    class="input input-bordered w-full"
                    pattern="[a-zA-Z0-9-]+"
                  />
                </div>

                <!-- Platform-specific URLs -->
                <div class="form-control">
                  <label class="label cursor-pointer select-text">
                    <span class="label-text font-medium">Platform-specific URLs</span>
                    <span class="label-text-alt text-base-content/60">Optional: Set different URLs for different platforms</span>
                  </label>

                  <!-- Android URL -->
                  <div class="mt-2">
                    <label class="label">
                      <span class="label-text">Android URL</span>
                      <span class="label-text-alt text-base-content/60">For Android devices</span>
                    </label>
                    <input
                      type="url"
                      v-model="formData.device_urls.android"
                      placeholder="URL for Android devices (e.g., Play Store link)"
                      class="input input-bordered w-full"
                    />
                  </div>

                  <!-- iOS URL -->
                  <div class="mt-2">
                    <label class="label">
                      <span class="label-text">iOS URL</span>
                      <span class="label-text-alt text-base-content/60">For iOS devices</span>
                    </label>
                    <input
                      type="url"
                      v-model="formData.device_urls.ios"
                      placeholder="URL for iOS devices (e.g., App Store link)"
                      class="input input-bordered w-full"
                    />
                  </div>

                  <!-- macOS URL -->
                  <div class="mt-2">
                    <label class="label">
                      <span class="label-text">macOS URL</span>
                      <span class="label-text-alt text-base-content/60">For macOS devices</span>
                    </label>
                    <input
                      type="url"
                      v-model="formData.device_urls.macos"
                      placeholder="URL for macOS devices (e.g., Mac App Store link)"
                      class="input input-bordered w-full"
                    />
                  </div>

                  <!-- Note about web fallback -->
                  <div class="mt-4 text-sm text-base-content/70">
                    <p>Note: The main URL above will be used as the default (web) fallback for all other platforms.</p>
                  </div>
                </div>

                <!-- Title with icon -->
                <div class="form-control">
                  <label class="label cursor-pointer select-text">
                    <span class="label-text font-medium">Link Title</span>
                  </label>
                  <div class="relative">
                    <input
                      type="text"
                      v-model="formData.title"
                      placeholder="Optional title for your link"
                      class="input input-bordered w-full pl-10"
                    />
                    <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5 absolute left-3 top-1/2 transform -translate-y-1/2 text-base-content/50" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                      <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11 5H6a2 2 0 00-2 2v11a2 2 0 002 2h11a2 2 0 002-2v-5m-1.414-9.414a2 2 0 112.828 2.828L11.828 15H9v-2.828l8.586-8.586z" />
                    </svg>
                  </div>
                </div>

                <!-- Expiry with badge -->
                <div class="form-control">
                  <label class="label cursor-pointer select-text">
                    <span class="label-text font-medium">Link Expiry</span>
                    <span v-if="formData.expiry_in_secs" class="badge badge-primary">Active</span>
                  </label>
                  <select v-model="formData.expiry_in_secs" class="select select-bordered w-full">
                    <option :value="null">Never expires</option>
                    <option :value="3600">1 Hour</option>
                    <option :value="86400">1 Day</option>
                    <option :value="604800">1 Week</option>
                    <option :value="2592000">30 Days</option>
                  </select>
                </div>
              </div>
            </div>
          </form>

          <!-- Result with animation -->
          <div v-if="shortUrl" class="mt-6 animate-fade-in">
            <div class="bg-success/10 rounded-box p-6 border border-success/20">
              <div class="flex items-center gap-4">
                <div class="flex-1">
                  <label class="label">
                    <span class="label-text font-medium text-success">Your shortened URL is ready!</span>
                  </label>
                  <div class="join w-full">
                    <input
                      type="text"
                      :value="shortUrl"
                      class="input join-item input-bordered w-full font-mono text-sm bg-base-100"
                      readonly
                    />
                    <button
                      @click="copyToClipboard"
                      class="btn join-item btn-success gap-2 min-w-[100px]"
                      :class="{'btn-success': copied, 'btn-outline': !copied}"
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path v-if="!copied" stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 5H6a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2v-1M8 5a2 2 0 002 2h2a2 2 0 002-2M8 5a2 2 0 012-2h2a2 2 0 012 2m0 0h2a2 2 0 012 2v3m2 4H10m0 0l3-3m-3 3l3 3" />
                        <path v-else stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
                      </svg>
                      <span>{{ copied ? 'Copied!' : 'Copy' }}</span>
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Error Message with shake animation -->
          <div v-if="error" class="mt-4 alert alert-error animate-shake">
            <svg xmlns="http://www.w3.org/2000/svg" class="stroke-current shrink-0 h-6 w-6" fill="none" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 14l2-2m0 0l2-2m-2 2l-2-2m2 2l2 2m7-2a9 9 0 11-18 0 9 9 0 0118 0z" />
            </svg>
            <span>{{ error }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'

const formData = ref({
  url: '',
  slug: '',
  title: '',
  expiry_in_secs: null,
  device_urls: {
    android: '',
    ios: '',
    macos: '',
    // web URL is the main URL
  }
})

const shortUrl = ref('')
const error = ref('')
const copied = ref(false)
const loading = ref(false)

async function handleSubmit() {
  error.value = ''
  copied.value = false
  loading.value = true

  try {
    const response = await fetch('/api/v1/shorten', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(formData.value),
    })

    const data = await response.json()
    if (data.status === 'success') {
      // Get public URL from API response metadata
      shortUrl.value = `${data.data.public_url}/${data.data.short_code}`
      // Reset form except URL
      formData.value.slug = ''
      formData.value.title = ''
      formData.value.expiry_in_secs = ''
    } else {
      error.value = data.message || 'Failed to create short URL'
    }
  } catch (err) {
    console.error('Error:', err)
    error.value = 'An unexpected error occurred'
  } finally {
    loading.value = false
  }
}

async function copyToClipboard() {
  try {
    await navigator.clipboard.writeText(shortUrl.value)
    copied.value = true
    setTimeout(() => {
      copied.value = false
    }, 2000)
  } catch (err) {
    console.error('Failed to copy:', err)
    error.value = 'Failed to copy to clipboard'
  }
}
</script>

<style>
.animate-fade-in {
  animation: fadeIn 0.3s ease-in;
}

.animate-shake {
  animation: shake 0.5s cubic-bezier(.36,.07,.19,.97) both;
}

@keyframes fadeIn {
  from { opacity: 0; transform: translateY(10px); }
  to { opacity: 1; transform: translateY(0); }
}

@keyframes shake {
  10%, 90% { transform: translate3d(-1px, 0, 0); }
  20%, 80% { transform: translate3d(2px, 0, 0); }
  30%, 50%, 70% { transform: translate3d(-4px, 0, 0); }
  40%, 60% { transform: translate3d(4px, 0, 0); }
}
</style>
