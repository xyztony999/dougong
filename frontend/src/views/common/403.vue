<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()

function goHome(): void {
  void router.replace('/')
}
</script>

<template>
  <div class="forbidden">
    <n-result
      status="403"
      title="无权限访问"
      description="当前角色为只读，无权执行变更操作。"
    >
      <template #footer>
        <n-button type="primary" @click="goHome">返回首页</n-button>
      </template>
    </n-result>
  </div>
</template>

<style scoped>
.forbidden {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 60vh;
}
</style>
