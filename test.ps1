# EasyMS 测试脚本 (PowerShell版本)

Write-Host "开始执行 EasyMS 测试..." -ForegroundColor Green

# 设置测试环境变量
$env:ENV = "test"

# 运行所有测试
Write-Host "运行所有测试..." -ForegroundColor Yellow
go test -v ./...

# 检查测试结果
if ($LASTEXITCODE -eq 0) {
    Write-Host "所有测试通过!" -ForegroundColor Green
} else {
    Write-Host "测试失败!" -ForegroundColor Red
    exit 1
}

# 运行覆盖率测试
Write-Host "运行测试覆盖率分析..." -ForegroundColor Yellow
go test -coverprofile=coverage.out ./...

# 检查覆盖率结果
if ($LASTEXITCODE -eq 0) {
    Write-Host "测试覆盖率报告已生成: coverage.out" -ForegroundColor Green
    go tool cover -html=coverage.out -o coverage.html
    Write-Host "HTML格式的覆盖率报告已生成: coverage.html" -ForegroundColor Green
} else {
    Write-Host "覆盖率测试失败!" -ForegroundColor Red
    exit 1
}

Write-Host "测试完成!" -ForegroundColor Green