@echo off
setlocal enabledelayedexpansion

echo ========================================
echo Тестирование RabbitMQ и Redis
echo ========================================

set RESULTS_DIR=.\results
if not exist %RESULTS_DIR% mkdir %RESULTS_DIR%

set BROKERS=rabbitmq redis
set SIZES=128 1024 10240 102400
set MESSAGES=5000
set DURATION=60

for %%b in (%BROKERS%) do (
    for %%s in (%SIZES%) do (
        echo.
        echo === Тестирование %%b с размером сообщения %%s байт ===
        
        rem Запуск consumer в отдельном окне (заголовок окна = Consumer_%%b_%%s)
        start "Consumer_%%b_%%s" /MIN cmd /c "python consumer.py --broker %%b --duration %DURATION% --queue test_queue > %RESULTS_DIR%\%%b_%%s_consumer.log 2>&1"
        
        rem Ожидание инициализации consumer
        timeout /t 3 /nobreak >nul
        
        rem Запуск producer (ждет завершения)
        python producer.py --broker %%b --messages %MESSAGES% --size %%s --queue test_queue
        
        rem Даем consumer время дообработать оставшиеся сообщения
        timeout /t 5 /nobreak >nul
        
        rem Закрываем окно consumer
        taskkill /fi "WindowTitle eq Consumer_%%b_%%s" /f >nul 2>&1
        
        rem Очистка очереди перед следующим тестом
        if "%%b"=="rabbitmq" (
            docker exec rabbitmq rabbitmqctl purge_queue test_queue >nul 2>&1
        ) else (
            docker exec redis redis-cli DEL test_queue >nul 2>&1
        )
        
        rem Пауза между тестами
        timeout /t 2 /nobreak >nul
    )
)

echo.
echo ========================================
echo Все тесты завершены! Результаты в папке %RESULTS_DIR%
echo ========================================
pause