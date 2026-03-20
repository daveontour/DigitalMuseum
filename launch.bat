@echo off
SET ROOT=%~dp0
SET PATH=%ROOT%bin\;%ROOT%bin\postgres\pgsql\bin;%ROOT%bin\imagemagick;%PATH%
SET MAGICK_HOME=%ROOT%bin\imagemagick

:: 1. Kill any "zombie" postgres processes
echo Checking for orphaned Postgres processes...
taskkill /f /im postgres.exe /t >nul 2>&1

:: 2. Initialize DB if data folder is empty
if not exist "%ROOT%data\PG_VERSION" (
    echo Initializing portable database with UTF8...
    "%ROOT%bin\postgres\pgsql\bin\initdb.exe" -D "%ROOT%data" -U postgres --auth=trust --encoding=UTF8 --locale=C
)

:: 3. Start Postgres
echo Starting Database...
"%ROOT%bin\postgres\pgsql\bin\pg_ctl.exe" start -D "%ROOT%data" -o "-p 5433 -c shared_buffers=128MB -c autovacuum_vacuum_cost_delay=20ms -c autovacuum_vacuum_cost_limit=200 -c log_checkpoints=off -c log_min_messages=warning"

:: 4. Run your App (FIXED: Added 'start' to run asynchronously)
echo Starting Application...
start "" "%ROOT%bin\digitalmuseum.exe" 

:: Wait for the server to initialize before opening the browser
timeout /t 5 /nobreak > nul

:: Open the browser
start "" "http://localhost:8001"


:: 5. Cleanup on exit
echo.
echo Press any key to shut down the application and database...
pause > nul

:: Kill the application process
echo Closing Application...
taskkill /f /im digitalmuseum.exe /t >nul 2>&1

:: Stop the database
echo Shutting down Database...
"%ROOT%bin\postgres\pgsql\bin\pg_ctl.exe" stop -D "%ROOT%data" -m fast

echo All systems shut down.
timeout /t 2 > nul
exit /b 0
