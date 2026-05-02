<template>
  <ProfileIdentityBindingsSection
    :user="user"
    :linuxdo-enabled="linuxdoEnabled"
    :oidc-enabled="oidcEnabled"
    :oidc-provider-name="oidcProviderName"
    :wechat-enabled="wechatEnabled"
    :wechat-open-enabled="wechatOpenEnabled"
    :wechat-mp-enabled="wechatMpEnabled"
  />
</template>

<script setup lang="ts">
import ProfileIdentityBindingsSection from '@/components/user/profile/ProfileIdentityBindingsSection.vue'
import type { User } from '@/types'

withDefaults(
  defineProps<{
    user: User | null
    linuxdoEnabled?: boolean
    oidcEnabled?: boolean
    oidcProviderName?: string
    wechatEnabled?: boolean
    wechatOpenEnabled?: boolean
    wechatMpEnabled?: boolean
  }>(),
  {
    linuxdoEnabled: false,
    oidcEnabled: false,
    oidcProviderName: 'OIDC',
    wechatEnabled: false,
    wechatOpenEnabled: undefined,
    wechatMpEnabled: undefined
  }
)
</script>
